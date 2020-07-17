// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/compare"
	"yunion.io/x/pkg/util/netutils"
	"yunion.io/x/pkg/utils"
	"yunion.io/x/sqlchemy"

	"yunion.io/x/onecloud/pkg/apis"
	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/consts"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/lockman"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/quotas"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/taskman"
	"yunion.io/x/onecloud/pkg/cloudprovider"
	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/util/rbacutils"
	"yunion.io/x/onecloud/pkg/util/stringutils2"
)

type SVpcManager struct {
	db.SEnabledStatusInfrasResourceBaseManager
	db.SExternalizedResourceBaseManager
	SManagedResourceBaseManager
	SCloudregionResourceBaseManager
	SGlobalVpcResourceBaseManager
}

var VpcManager *SVpcManager

func init() {
	VpcManager = &SVpcManager{
		SEnabledStatusInfrasResourceBaseManager: db.NewEnabledStatusInfrasResourceBaseManager(
			SVpc{},
			"vpcs_tbl",
			"vpc",
			"vpcs",
		),
	}
	VpcManager.SetVirtualObject(VpcManager)
}

type SVpc struct {
	db.SEnabledStatusInfrasResourceBase
	db.SExternalizedResourceBase

	SManagedResourceBase

	SCloudregionResourceBase `width:"36" charset:"ascii" nullable:"false" list:"domain" create:"domain_required" default:"default"`

	SGlobalVpcResourceBase `width:"36" charset:"ascii" list:"user" json:"globalvpc_id"`

	// 是否是默认VPC
	// example: true
	IsDefault bool `default:"false" list:"domain" create:"domain_optional"`

	// CIDR地址段
	// example: 192.168.222.0/24
	CidrBlock string `charset:"ascii" nullable:"true" list:"domain" create:"domain_optional"`

	// Vpc外网访问模式
	ExternalAccessMode string `width:"16" charset:"ascii" nullable:"true" list:"user" update:"user" create:"optional"`
}

func (manager *SVpcManager) GetContextManagers() [][]db.IModelManager {
	return [][]db.IModelManager{
		{CloudregionManager},
		{GlobalVpcManager},
	}
}

func (self *SVpc) GetCloudRegionId() string {
	if len(self.CloudregionId) == 0 {
		return api.DEFAULT_REGION_ID
	} else {
		return self.CloudregionId
	}
}

func (self *SVpc) CustomizeCreate(ctx context.Context, userCred mcclient.TokenCredential, ownerId mcclient.IIdentityProvider, query jsonutils.JSONObject, data jsonutils.JSONObject) error {
	idstr, _ := data.GetString("id")
	if len(idstr) > 0 {
		self.Id = idstr
	}
	return self.SEnabledStatusInfrasResourceBase.CustomizeCreate(ctx, userCred, ownerId, query, data)
}

func (self *SVpc) getNatgatewayQuery() *sqlchemy.SQuery {
	return NatGatewayManager.Query().Equals("vpc_id", self.Id)
}

func (self *SVpc) GetNatgatewayCount() (int, error) {
	return self.getNatgatewayQuery().CountWithError()
}

func (self *SVpc) GetNatgateways() ([]SNatGateway, error) {
	nats := []SNatGateway{}
	err := db.FetchModelObjects(NatGatewayManager, self.getNatgatewayQuery(), &nats)
	if err != nil {
		return nil, err
	}
	return nats, nil
}

func (self *SVpc) ValidateUpdateData(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, input api.VpcUpdateInput) (api.VpcUpdateInput, error) {
	if input.ExternalAccessMode != "" {
		if !utils.IsInStringArray(input.ExternalAccessMode, api.VPC_EXTERNAL_ACCESS_MODES) {
			return input, httperrors.NewInputParameterError("invalid external_access_mode %q, want %s",
				input.ExternalAccessMode, api.VPC_EXTERNAL_ACCESS_MODES)
		}
	}
	if _, err := self.SEnabledStatusInfrasResourceBase.ValidateUpdateData(ctx, userCred, query, input.EnabledStatusInfrasResourceBaseUpdateInput); err != nil {
		return input, err
	}
	return input, nil
}

func (self *SVpc) ValidateDeleteCondition(ctx context.Context) error {
	cnt, err := self.GetNetworkCount()
	if err != nil {
		return httperrors.NewInternalServerError("GetNetworkCount fail %s", err)
	}
	if cnt > 0 {
		return httperrors.NewNotEmptyError("VPC not empty, please delete network first")
	}
	cnt, err = self.GetNatgatewayCount()
	if err != nil {
		return httperrors.NewInternalServerError("GetNatgatewayCount fail %v", err)
	}
	if cnt > 0 {
		return httperrors.NewNotEmptyError("VPC not empty, please delete nat gateway first")
	}
	if self.Id == api.DEFAULT_VPC_ID {
		return httperrors.NewProtectedResourceError("not allow to delete default vpc")
	}
	return self.SEnabledStatusInfrasResourceBase.ValidateDeleteCondition(ctx)
}

func (self *SVpc) getWireQuery() *sqlchemy.SQuery {
	wires := WireManager.Query()
	if self.Id == api.DEFAULT_VPC_ID {
		return wires.Filter(sqlchemy.OR(sqlchemy.IsNull(wires.Field("vpc_id")),
			sqlchemy.IsEmpty(wires.Field("vpc_id")),
			sqlchemy.Equals(wires.Field("vpc_id"), self.Id)))
	} else {
		return wires.Equals("vpc_id", self.Id)
	}
}

func (self *SVpc) GetWireCount() (int, error) {
	q := self.getWireQuery()
	return q.CountWithError()
}

func (self *SVpc) GetWires() []SWire {
	wires := make([]SWire, 0)
	q := self.getWireQuery()
	err := db.FetchModelObjects(WireManager, q, &wires)
	if err != nil {
		log.Errorf("getWires fail %s", err)
		return nil
	}
	return wires
}

func (manager *SVpcManager) getVpcExternalIdForClassicNetwork(regionId, cloudproviderId string) string {
	return fmt.Sprintf("%s-%s", regionId, cloudproviderId)
}

func (manager *SVpcManager) GetOrCreateVpcForClassicNetwork(ctx context.Context, region *SCloudregion) (*SVpc, error) {
	cloudprovider := region.GetCloudprovider()
	externalId := manager.getVpcExternalIdForClassicNetwork(region.Id, cloudprovider.Id)
	_vpc, err := db.FetchByExternalIdAndManagerId(manager, externalId, func(q *sqlchemy.SQuery) *sqlchemy.SQuery {
		return q.Equals("manager_id", region.ManagerId)
	})
	if err == nil {
		return _vpc.(*SVpc), nil
	}
	if errors.Cause(err) != sql.ErrNoRows {
		return nil, errors.Wrap(err, "db.FetchByExternalId")
	}
	vpc := &SVpc{}
	vpc.IsDefault = false
	vpc.CloudregionId = region.Id
	vpc.SetModelManager(manager, vpc)
	vpc.Name = fmt.Sprintf("emulated vpc for %s %s classic network", region.Name, cloudprovider.Name)
	vpc.IsEmulated = true
	vpc.SetEnabled(false)
	vpc.Status = api.VPC_STATUS_UNAVAILABLE
	vpc.ExternalId = externalId
	vpc.ManagerId = region.ManagerId
	err = manager.TableSpec().Insert(ctx, vpc)
	if err != nil {
		return nil, errors.Wrap(err, "Insert vpc for classic network")
	}
	return vpc, nil
}

func (self *SVpc) getNetworkQuery() *sqlchemy.SQuery {
	q := NetworkManager.Query()
	wireQ := self.getWireQuery().SubQuery()
	q = q.In("wire_id", wireQ.Query(wireQ.Field("id")).SubQuery())
	return q
}

func (self *SVpc) GetNetworkCount() (int, error) {
	q := self.getNetworkQuery()
	return q.CountWithError()
}

func (self *SVpc) GetRouteTableQuery() *sqlchemy.SQuery {
	return RouteTableManager.Query().Equals("vpc_id", self.Id)
}

func (self *SVpc) GetRouteTables() []SRouteTable {
	q := self.GetRouteTableQuery()
	routes := []SRouteTable{}
	db.FetchModelObjects(RouteTableManager, q, &routes)
	return routes
}

func (self *SVpc) GetRouteTableCount() (int, error) {
	return self.GetRouteTableQuery().CountWithError()
}

func (self *SVpc) getMoreDetails(out api.VpcDetails) api.VpcDetails {
	out.WireCount, _ = self.GetWireCount()
	out.NetworkCount, _ = self.GetNetworkCount()
	out.RoutetableCount, _ = self.GetRouteTableCount()
	out.NatgatewayCount, _ = self.GetNatgatewayCount()
	return out
}

func (self *SVpc) getCloudProviderInfo() SCloudProviderInfo {
	region, _ := self.GetRegion()
	provider := self.GetCloudprovider()
	return MakeCloudProviderInfo(region, nil, provider)
}

func (self *SVpc) GetRegion() (*SCloudregion, error) {
	region, err := CloudregionManager.FetchById(self.CloudregionId)
	if err != nil {
		return nil, errors.Wrap(err, "CloudregionManager.FetchById")
	}
	return region.(*SCloudregion), nil
}

func (self *SVpc) getZoneByExternalId(externalId string) (*SZone, error) {
	region, err := self.GetRegion()
	if err != nil {
		return nil, errors.Wrapf(err, "getZoneByExternalId.GetRegion")
	}
	zones := []SZone{}
	q := ZoneManager.Query().Equals("cloudregion_id", region.Id).Equals("external_id", externalId)
	err = db.FetchModelObjects(ZoneManager, q, &zones)
	if err != nil {
		return nil, errors.Wrapf(err, "getZoneByExternalId.FetchModelObjects")
	}
	if len(zones) == 1 {
		return &zones[0], nil
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("failed to found zone by externalId %s in cloudregion %s(%s)", externalId, region.Name, region.Id)
	}
	return nil, fmt.Errorf("found %d duplicate zones by externalId %s in cloudregion %s(%s)", len(zones), externalId, region.Name, region.Id)
}

func (self *SVpc) GetExtraDetails(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	query jsonutils.JSONObject,
	isList bool,
) (api.VpcDetails, error) {
	return api.VpcDetails{}, nil
}

func (manager *SVpcManager) FetchCustomizeColumns(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	query jsonutils.JSONObject,
	objs []interface{},
	fields stringutils2.SSortedStrings,
	isList bool,
) []api.VpcDetails {
	rows := make([]api.VpcDetails, len(objs))
	stdRows := manager.SEnabledStatusInfrasResourceBaseManager.FetchCustomizeColumns(ctx, userCred, query, objs, fields, isList)
	managerRows := manager.SManagedResourceBaseManager.FetchCustomizeColumns(ctx, userCred, query, objs, fields, isList)
	regionRows := manager.SCloudregionResourceBaseManager.FetchCustomizeColumns(ctx, userCred, query, objs, fields, isList)
	globalVpcRows := manager.SGlobalVpcResourceBaseManager.FetchCustomizeColumns(ctx, userCred, query, objs, fields, isList)
	for i := range rows {
		rows[i] = api.VpcDetails{
			EnabledStatusInfrasResourceBaseDetails: stdRows[i],
			ManagedResourceInfo:                    managerRows[i],
			CloudregionResourceInfo:                regionRows[i],
			GlobalVpcResourceInfo:                  globalVpcRows[i],
		}
		rows[i] = objs[i].(*SVpc).getMoreDetails(rows[i])
	}
	return rows
}

func (self *SVpc) setDefault(def bool) error {
	var err error
	if self.IsDefault != def {
		_, err = db.Update(self, func() error {
			self.IsDefault = def
			return nil
		})
	}
	return err
}

func (manager *SVpcManager) SyncVPCs(ctx context.Context, userCred mcclient.TokenCredential, provider *SCloudprovider, region *SCloudregion, vpcs []cloudprovider.ICloudVpc) ([]SVpc, []cloudprovider.ICloudVpc, compare.SyncResult) {
	lockman.LockClass(ctx, manager, db.GetLockClassKey(manager, userCred))
	defer lockman.ReleaseClass(ctx, manager, db.GetLockClassKey(manager, userCred))

	localVPCs := make([]SVpc, 0)
	remoteVPCs := make([]cloudprovider.ICloudVpc, 0)
	syncResult := compare.SyncResult{}

	dbVPCs, err := region.GetCloudproviderVpcs(provider.Id)
	if err != nil {
		syncResult.Error(err)
		return nil, nil, syncResult
	}

	for i := range dbVPCs {
		if taskman.TaskManager.IsInTask(&dbVPCs[i]) {
			syncResult.Error(fmt.Errorf("object in task"))
			return nil, nil, syncResult
		}
	}

	removed := make([]SVpc, 0)
	commondb := make([]SVpc, 0)
	commonext := make([]cloudprovider.ICloudVpc, 0)
	added := make([]cloudprovider.ICloudVpc, 0)

	err = compare.CompareSets(dbVPCs, vpcs, &removed, &commondb, &commonext, &added)
	if err != nil {
		syncResult.Error(err)
		return nil, nil, syncResult
	}

	for i := 0; i < len(removed); i += 1 {
		err = removed[i].syncRemoveCloudVpc(ctx, userCred)
		if err != nil {
			syncResult.DeleteError(err)
		} else {
			syncResult.Delete()
		}
	}
	for i := 0; i < len(commondb); i += 1 {
		err = commondb[i].SyncWithCloudVpc(ctx, userCred, commonext[i], provider)
		if err != nil {
			syncResult.UpdateError(err)
			continue
		}
		syncMetadata(ctx, userCred, &commondb[i], commonext[i])
		localVPCs = append(localVPCs, commondb[i])
		remoteVPCs = append(remoteVPCs, commonext[i])
		syncResult.Update()
		err = commondb[i].SyncGlobalVpc(ctx, userCred, provider.GetOwnerId())
		if err != nil {
			log.Errorf("%s(%s) sync global vpc error: %v", commondb[i].Name, commondb[i].Id, err)
		}
	}
	for i := 0; i < len(added); i += 1 {
		newVpc, err := manager.newFromCloudVpc(ctx, userCred, added[i], provider, region)
		if err != nil {
			syncResult.AddError(err)
			continue
		}
		syncMetadata(ctx, userCred, newVpc, added[i])
		localVPCs = append(localVPCs, *newVpc)
		remoteVPCs = append(remoteVPCs, added[i])
		syncResult.Add()
		err = newVpc.SyncGlobalVpc(ctx, userCred, provider.GetOwnerId())
		if err != nil {
			log.Errorf("%s(%s) sync global vpc error: %v", newVpc.Name, newVpc.Id, err)
		}
	}

	return localVPCs, remoteVPCs, syncResult
}

func (self *SVpc) syncRemoveCloudVpc(ctx context.Context, userCred mcclient.TokenCredential) error {
	lockman.LockObject(ctx, self)
	defer lockman.ReleaseObject(ctx, self)

	if VpcManager.getVpcExternalIdForClassicNetwork(self.CloudregionId, self.ManagerId) == self.ExternalId { //为经典网络虚拟的vpc
		return nil
	}

	err := self.ValidateDeleteCondition(ctx)
	if err != nil { // cannot delete
		self.markAllNetworksUnknown(userCred)
		_, err = self.PerformDisable(ctx, userCred, nil, apis.PerformDisableInput{})
		if err == nil {
			err = self.SetStatus(userCred, api.VPC_STATUS_UNKNOWN, "sync to delete")
		}
	} else {
		err = self.RealDelete(ctx, userCred)
	}
	return err
}

func (self *SVpc) SyncGlobalVpc(ctx context.Context, userCred mcclient.TokenCredential, ownerId mcclient.IIdentityProvider) error {
	if len(self.GlobalvpcId) > 0 {
		gv, _ := self.GetGlobalVpc()
		SyncCloudDomain(userCred, gv, ownerId)
		return nil
	}
	region, err := self.GetRegion()
	if err != nil {
		return errors.Wrap(err, "GetRegion")
	}
	if region.GetDriver().IsVpcBelongGlobalVpc() {
		externalId := strings.Replace(self.ExternalId, region.ExternalId+"/", "", -1)
		vpcs := []SVpc{}
		sq := VpcManager.Query().SubQuery()
		q := sq.Query().Filter(
			sqlchemy.AND(
				sqlchemy.Equals(sq.Field("manager_id"), self.ManagerId),
				sqlchemy.NOT(sqlchemy.IsNullOrEmpty(sq.Field("globalvpc_id"))),
				sqlchemy.Endswith(sq.Field("external_id"), externalId),
			),
		)
		err := db.FetchModelObjects(VpcManager, q, &vpcs)
		if err != nil {
			return errors.Wrap(err, "db.FetchModelObjects")
		}
		globalvpcId := ""
		if len(vpcs) > 0 {
			globalvpcId = vpcs[0].GlobalvpcId
		} else {
			gv := &SGlobalVpc{}
			gv.Name = self.Name
			idx := strings.Index(gv.Name, "(")
			if idx > 0 {
				gv.Name = gv.Name[:idx]
			}
			gv.Name, err = db.GenerateName(GlobalVpcManager, userCred, gv.Name)
			if err != nil {
				return errors.Wrap(err, "db.GenerateName")
			}
			gv.SetEnabled(true)
			gv.Status = api.GLOBAL_VPC_STATUS_AVAILABLE
			gv.SetModelManager(GlobalVpcManager, gv)
			err = GlobalVpcManager.TableSpec().Insert(ctx, gv)
			if err != nil {
				return errors.Wrap(err, "GlobalVpcManager.Insert")
			}
			SyncCloudDomain(userCred, gv, ownerId)
			globalvpcId = gv.Id
		}
		_, err = db.Update(self, func() error {
			self.GlobalvpcId = globalvpcId
			return nil
		})
		return err
	}
	return nil
}

func (self *SVpc) SyncWithCloudVpc(ctx context.Context, userCred mcclient.TokenCredential, extVPC cloudprovider.ICloudVpc, provider *SCloudprovider) error {
	diff, err := db.UpdateWithLock(ctx, self, func() error {
		extVPC.Refresh()
		// self.Name = extVPC.GetName()
		self.Status = extVPC.GetStatus()
		self.CidrBlock = extVPC.GetCidrBlock()
		self.IsDefault = extVPC.GetIsDefault()
		self.ExternalId = extVPC.GetGlobalId()

		self.IsEmulated = extVPC.IsEmulated()

		return nil
	})
	if err != nil {
		return err
	}

	if provider != nil {
		SyncCloudDomain(userCred, self, provider.GetOwnerId())
		self.SyncShareState(ctx, userCred, provider.getAccountShareInfo())
	}

	db.OpsLog.LogSyncUpdate(self, diff, userCred)
	return nil
}

func (manager *SVpcManager) newFromCloudVpc(ctx context.Context, userCred mcclient.TokenCredential, extVPC cloudprovider.ICloudVpc, provider *SCloudprovider, region *SCloudregion) (*SVpc, error) {
	vpc := SVpc{}
	vpc.SetModelManager(manager, &vpc)

	newName, err := db.GenerateName(manager, userCred, extVPC.GetName())
	if err != nil {
		return nil, err
	}
	vpc.Name = newName
	vpc.Status = extVPC.GetStatus()
	vpc.ExternalId = extVPC.GetGlobalId()
	vpc.IsDefault = extVPC.GetIsDefault()
	vpc.CidrBlock = extVPC.GetCidrBlock()
	vpc.CloudregionId = region.Id

	vpc.ManagerId = provider.Id

	vpc.IsEmulated = extVPC.IsEmulated()

	err = manager.TableSpec().Insert(ctx, &vpc)
	if err != nil {
		log.Errorf("newFromCloudVpc fail %s", err)
		return nil, err
	}

	SyncCloudDomain(userCred, &vpc, provider.GetOwnerId())

	if provider != nil {
		vpc.SyncShareState(ctx, userCred, provider.getAccountShareInfo())
	}

	db.OpsLog.LogEvent(&vpc, db.ACT_CREATE, vpc.GetShortDesc(ctx), userCred)

	return &vpc, nil
}

func (self *SVpc) markAllNetworksUnknown(userCred mcclient.TokenCredential) error {
	wires := self.GetWires()
	if wires == nil || len(wires) == 0 {
		return nil
	}
	for i := 0; i < len(wires); i += 1 {
		wires[i].markNetworkUnknown(userCred)
	}
	return nil
}

func (manager *SVpcManager) InitializeData() error {
	if vpcObj, err := manager.FetchById(api.DEFAULT_VPC_ID); err != nil {
		if err == sql.ErrNoRows {
			defVpc := SVpc{}
			defVpc.SetModelManager(VpcManager, &defVpc)

			defVpc.Id = api.DEFAULT_VPC_ID
			defVpc.Name = "Default"
			defVpc.CloudregionId = api.DEFAULT_REGION_ID
			defVpc.Description = "Default VPC"
			defVpc.Status = api.VPC_STATUS_AVAILABLE
			defVpc.IsDefault = true
			defVpc.IsPublic = true
			defVpc.PublicScope = string(rbacutils.ScopeSystem)
			err = manager.TableSpec().Insert(context.TODO(), &defVpc)
			if err != nil {
				log.Errorf("Insert default vpc fail: %s", err)
			}
			return err
		} else {
			return err
		}
	} else {
		vpc := vpcObj.(*SVpc)
		if vpc.Status != api.VPC_STATUS_AVAILABLE || (vpc.PublicScope == string(rbacutils.ScopeSystem) && !vpc.IsPublic) {
			_, err = db.Update(vpc, func() error {
				vpc.Status = api.VPC_STATUS_AVAILABLE
				vpc.IsPublic = true
				return nil
			})
			return err
		}
	}

	{ // initialize default external_access_mode for onecloud vpc
		var vpcs []SVpc
		q := manager.Query().
			IsNullOrEmpty("manager_id").
			IsNullOrEmpty("external_id").
			IsNullOrEmpty("external_access_mode")
		if err := db.FetchModelObjects(manager, q, &vpcs); err != nil {
			return errors.Wrap(err, "fetch onecloud vpc with external_access_mode not set")
		}
		for i := range vpcs {
			vpc := &vpcs[i]
			if _, err := db.Update(vpc, func() error {
				vpc.ExternalAccessMode = api.VPC_EXTERNAL_ACCESS_MODE_EIP_DISTGW
				return nil
			}); err != nil {
				return errors.Wrap(err, "db set default external_access_mode")
			}
		}
	}

	return nil
}

func (manager *SVpcManager) ValidateCreateData(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	ownerId mcclient.IIdentityProvider,
	query jsonutils.JSONObject,
	input api.VpcCreateInput,
) (api.VpcCreateInput, error) {
	regionId := input.Cloudregion
	if len(regionId) == 0 {
		return input, httperrors.NewMissingParameterError("cloudregion_id")
	}
	regionObj, err := CloudregionManager.FetchByIdOrName(userCred, regionId)
	if err != nil {
		if err == sql.ErrNoRows {
			return input, httperrors.NewResourceNotFoundError2(CloudregionManager.Keyword(), regionId)
		} else {
			return input, httperrors.NewGeneralError(err)
		}
	}
	region := regionObj.(*SCloudregion)
	input.Cloudregion = region.Id
	// data.Add(jsonutils.NewString(region.GetId()), "cloudregion_id")
	if region.isManaged() {
		managerStr := input.Cloudprovider
		if len(managerStr) == 0 {
			return input, httperrors.NewMissingParameterError("manager_id")
		}
		managerObj, err := CloudproviderManager.FetchByIdOrName(userCred, managerStr)
		if err != nil {
			if err == sql.ErrNoRows {
				return input, httperrors.NewResourceNotFoundError2(CloudproviderManager.Keyword(), managerStr)
			} else {
				return input, httperrors.NewGeneralError(err)
			}
		}
		input.Cloudprovider = managerObj.GetId()
		// data.Add(jsonutils.NewString(managerObj.GetId()), "manager_id")
	} else {
		input.Status = api.VPC_STATUS_AVAILABLE
		if input.ExternalAccessMode == "" {
			input.ExternalAccessMode = api.VPC_EXTERNAL_ACCESS_MODE_EIP_DISTGW
		}
		if !utils.IsInStringArray(input.ExternalAccessMode, api.VPC_EXTERNAL_ACCESS_MODES) {
			return input, httperrors.NewInputParameterError("invalid external_access_mode %q, want %s",
				input.Status, api.VPC_EXTERNAL_ACCESS_MODES)
		}
	}

	cidrBlock := input.CidrBlock
	if len(cidrBlock) > 0 {
		blocks := strings.Split(cidrBlock, ",")
		for _, block := range blocks {
			_, err = netutils.NewIPV4Prefix(block)
			if err != nil {
				return input, httperrors.NewInputParameterError("invalid cidr_block %s", cidrBlock)
			}
		}
	}

	input.EnabledStatusInfrasResourceBaseCreateInput, err = manager.SEnabledStatusInfrasResourceBaseManager.ValidateCreateData(ctx, userCred, ownerId, query, input.EnabledStatusInfrasResourceBaseCreateInput)
	if err != nil {
		return input, err
	}

	input, err = region.GetDriver().ValidateCreateVpcData(ctx, userCred, input)
	if err != nil {
		return input, errors.Wrapf(err, "region.GetDriver().ValidateCreateVpcData")
	}

	if region.GetDriver().IsVpcCreateNeedInputCidr() && len(input.CidrBlock) == 0 {
		return input, httperrors.NewMissingParameterError("cidr")
	}

	keys := GetVpcQuotaKeysFromCreateInput(input)
	quota := &SInfrasQuota{Vpc: 1}
	quota.SetKeys(keys)
	err = quotas.CheckSetPendingQuota(ctx, userCred, quota)
	if err != nil {
		return input, errors.Wrap(err, "quotas.CheckSetPendingQuota")
	}

	return input, nil
}

func (self *SVpc) PostCreate(ctx context.Context, userCred mcclient.TokenCredential, ownerId mcclient.IIdentityProvider, query jsonutils.JSONObject, data jsonutils.JSONObject) {
	input := api.VpcCreateInput{}
	err := data.Unmarshal(&input)
	if err != nil {
		log.Errorf("input unmarshal error %s", err)
	} else {
		pendingUsage := &SInfrasQuota{Vpc: 1}
		keys := GetVpcQuotaKeysFromCreateInput(input)
		pendingUsage.SetKeys(keys)
		quotas.CancelPendingUsage(ctx, userCred, pendingUsage, pendingUsage, true)
	}

	if len(self.ManagerId) == 0 {
		return
	}
	task, err := taskman.TaskManager.NewTask(ctx, "VpcCreateTask", self, userCred, nil, "", "", nil)
	if err != nil {
		log.Errorf("VpcCreateTask newTask error %s", err)
	} else {
		task.ScheduleRun(nil)
	}
}

func (self *SVpc) GetIRegion() (cloudprovider.ICloudRegion, error) {
	region, err := self.GetRegion()
	if err != nil {
		return nil, errors.Wrap(err, "GetRegion")
	}
	provider, err := self.GetDriver()
	if err != nil {
		return nil, err
	}
	return provider.GetIRegionById(region.GetExternalId())
}

func (self *SVpc) GetIVpc() (cloudprovider.ICloudVpc, error) {
	provider, err := self.GetDriver()
	if err != nil {
		log.Errorf("fail to find cloud provider")
		return nil, err
	}
	var iregion cloudprovider.ICloudRegion
	if provider.GetFactory().IsOnPremise() {
		iregion, err = provider.GetOnPremiseIRegion()
	} else {
		region, err := self.GetRegion()
		if err != nil {
			return nil, err
		}
		iregion, err = provider.GetIRegionById(region.ExternalId)
	}
	if err != nil {
		log.Errorf("fail to find iregion: %s", err)
		return nil, err
	}
	ivpc, err := iregion.GetIVpcById(self.ExternalId)
	if err != nil {
		log.Errorf("fail to find ivpc by id %s %s", self.ExternalId, err)
		return nil, err
	}
	return ivpc, nil
}

func (self *SVpc) Delete(ctx context.Context, userCred mcclient.TokenCredential) error {
	log.Infof("SVpc delete do nothing")
	self.SetStatus(userCred, api.VPC_STATUS_START_DELETE, "")
	return nil
}

func (self *SVpc) CustomizeDelete(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) error {
	if self.Id != api.DEFAULT_VPC_ID {
		return self.StartDeleteVpcTask(ctx, userCred)
	} else {
		return self.RealDelete(ctx, userCred)
	}
}

func (self *SVpc) RealDelete(ctx context.Context, userCred mcclient.TokenCredential) error {
	db.OpsLog.LogEvent(self, db.ACT_DELOCATE, self.GetShortDesc(ctx), userCred)
	self.SetStatus(userCred, api.VPC_STATUS_DELETED, "real delete")
	routes := self.GetRouteTables()
	var err error
	for i := 0; i < len(routes); i++ {
		err = routes[i].RealDelete(ctx, userCred)
		if err != nil {
			return errors.Wrapf(err, "delete route table %s failed", routes[i].GetId())
		}
	}
	natgateways, err := self.GetNatgateways()
	if err != nil {
		return errors.Wrap(err, "fetch natgateways failed")
	}
	for i := range natgateways {
		err = natgateways[i].RealDelete(ctx, userCred)
		if err != nil {
			return errors.Wrapf(err, "delete natgateway %s failed", natgateways[i].GetId())
		}
	}

	return self.SEnabledStatusInfrasResourceBase.Delete(ctx, userCred)
}

func (self *SVpc) StartDeleteVpcTask(ctx context.Context, userCred mcclient.TokenCredential) error {
	task, err := taskman.TaskManager.NewTask(ctx, "VpcDeleteTask", self, userCred, nil, "", "", nil)
	if err != nil {
		log.Errorf("Start vpcdeleteTask fail %s", err)
		return err
	}
	task.ScheduleRun(nil)
	return nil
}

func (self *SVpc) getPrefix() []netutils.IPV4Prefix {
	if len(self.CidrBlock) > 0 {
		ret := []netutils.IPV4Prefix{}
		blocks := strings.Split(self.CidrBlock, ",")
		for _, block := range blocks {
			prefix, _ := netutils.NewIPV4Prefix(block)
			ret = append(ret, prefix)
		}
		return ret
	}
	return []netutils.IPV4Prefix{{}}
}

func (self *SVpc) getIPRanges() []netutils.IPV4AddrRange {
	ret := []netutils.IPV4AddrRange{}
	prefs := self.getPrefix()
	for _, pref := range prefs {
		ret = append(ret, pref.ToIPRange())
	}

	return ret
}

func (self *SVpc) containsIPV4Range(a netutils.IPV4AddrRange) bool {
	ranges := self.getIPRanges()
	for i := range ranges {
		if ranges[i].ContainsRange(a) {
			return true
		}
	}
	return false
}

func (self *SVpc) AllowPerformPurge(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) bool {
	return db.IsAdminAllowPerform(userCred, self, "purge")
}

func (self *SVpc) PerformPurge(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	err := self.ValidateDeleteCondition(ctx)
	if err != nil {
		return nil, err
	}
	provider := self.GetCloudprovider()
	if provider != nil {
		if provider.GetEnabled() {
			return nil, httperrors.NewInvalidStatusError("Cannot purge vpc on enabled cloud provider")
		}
	}
	err = self.RealDelete(ctx, userCred)
	return nil, err
}

// 列出VPC
func (manager *SVpcManager) ListItemFilter(
	ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query api.VpcListInput,
) (*sqlchemy.SQuery, error) {
	var err error

	q, err = manager.SEnabledStatusInfrasResourceBaseManager.ListItemFilter(ctx, q, userCred, query.EnabledStatusInfrasResourceBaseListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SEnabledStatusInfrasResourceBaseManager.ListItemFilter")
	}

	q, err = manager.SExternalizedResourceBaseManager.ListItemFilter(ctx, q, userCred, query.ExternalizedResourceBaseListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SExternalizedResourceBaseManager.ListItemFilter")
	}

	q, err = manager.SManagedResourceBaseManager.ListItemFilter(ctx, q, userCred, query.ManagedResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SManagedResourceBaseManager.ListItemFilter")
	}

	q, err = manager.SCloudregionResourceBaseManager.ListItemFilter(ctx, q, userCred, query.RegionalFilterListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SCloudregionResourceBaseManager.ListItemFilter")
	}

	q, err = manager.SGlobalVpcResourceBaseManager.ListItemFilter(ctx, q, userCred, query.GlobalVpcResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SGlobalVpcResourceBaseManager.ListItemFilter")
	}

	usable := (query.Usable != nil && *query.Usable)
	vpcUsable := (query.UsableVpc != nil && *query.UsableVpc)
	if vpcUsable || usable {
		regions := CloudregionManager.Query().SubQuery()
		cloudproviders := CloudproviderManager.Query().SubQuery()
		providerSQ := cloudproviders.Query(cloudproviders.Field("id")).Filter(
			sqlchemy.AND(
				sqlchemy.IsTrue(cloudproviders.Field("enabled")),
				sqlchemy.In(cloudproviders.Field("status"), api.CLOUD_PROVIDER_VALID_STATUS),
				sqlchemy.In(cloudproviders.Field("health_status"), api.CLOUD_PROVIDER_VALID_HEALTH_STATUS),
			),
		)
		q = q.Join(regions, sqlchemy.Equals(q.Field("cloudregion_id"), regions.Field("id"))).Filter(
			sqlchemy.AND(
				sqlchemy.Equals(regions.Field("status"), api.CLOUD_REGION_STATUS_INSERVER),
				sqlchemy.OR(
					sqlchemy.In(q.Field("manager_id"), providerSQ.SubQuery()),
					sqlchemy.IsNullOrEmpty(q.Field("manager_id")),
				),
			),
		)

		if usable {
			wires := WireManager.Query().SubQuery()
			networks := NetworkManager.Query().SubQuery()

			sq := wires.Query(wires.Field("vpc_id")).Join(networks, sqlchemy.Equals(wires.Field("id"), networks.Field("wire_id"))).Filter(
				sqlchemy.Equals(networks.Field("status"), api.NETWORK_STATUS_AVAILABLE),
			)

			q = q.In("id", sq.SubQuery())
		}
	}

	if query.IsDefault != nil {
		if *query.IsDefault {
			q = q.IsTrue("is_default")
		} else {
			q = q.IsFalse("is_default")
		}
	}
	if len(query.CidrBlock) > 0 {
		q = q.In("cidr_block", query.CidrBlock)
	}

	return q, nil
}

func (manager *SVpcManager) QueryDistinctExtraField(q *sqlchemy.SQuery, field string) (*sqlchemy.SQuery, error) {
	switch field {
	case "vpc":
		q = q.AppendField(q.Field("name").Label("vpc")).Distinct()
		return q, nil
	default:
		var err error
		q, err = manager.SEnabledStatusInfrasResourceBaseManager.QueryDistinctExtraField(q, field)
		if err == nil {
			return q, nil
		}

		q, err = manager.SManagedResourceBaseManager.QueryDistinctExtraField(q, field)
		if err == nil {
			return q, nil
		}

		q, err = manager.SCloudregionResourceBaseManager.QueryDistinctExtraField(q, field)
		if err == nil {
			return q, nil
		}

		q, err = manager.SGlobalVpcResourceBaseManager.QueryDistinctExtraField(q, field)
		if err == nil {
			return q, nil
		}
	}
	return q, httperrors.ErrNotFound
}

func (manager *SVpcManager) OrderByExtraFields(
	ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query api.VpcListInput,
) (*sqlchemy.SQuery, error) {
	q, err := manager.SEnabledStatusInfrasResourceBaseManager.OrderByExtraFields(ctx, q, userCred, query.EnabledStatusInfrasResourceBaseListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SEnabledStatusInfrasResourceBaseManager.OrderByExtraFields")
	}
	q, err = manager.SManagedResourceBaseManager.OrderByExtraFields(ctx, q, userCred, query.ManagedResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SManagedResourceBaseManager.OrderByExtraFields")
	}
	q, err = manager.SCloudregionResourceBaseManager.OrderByExtraFields(ctx, q, userCred, query.RegionalFilterListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SCloudregionResourceBaseManager.OrderByExtraFields")
	}
	q, err = manager.SGlobalVpcResourceBaseManager.OrderByExtraFields(ctx, q, userCred, query.GlobalVpcResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SGlobalVpcResourceBaseManager.OrderByExtraFields")
	}
	return q, nil
}

func (self *SVpc) SyncRemoteWires(ctx context.Context, userCred mcclient.TokenCredential) error {
	ivpc, err := self.GetIVpc()
	if err != nil {
		return err
	}

	provider := CloudproviderManager.FetchCloudproviderById(self.ManagerId)
	syncVpcWires(ctx, userCred, nil, provider, self, ivpc, &SSyncRange{})

	hosts := HostManager.GetHostsByManagerAndRegion(provider.Id, self.CloudregionId)
	for i := 0; i < len(hosts); i += 1 {
		ihost, err := hosts[i].GetIHost()
		if err != nil {
			return err
		}
		syncHostWires(ctx, userCred, nil, provider, &hosts[i], ihost)
	}
	return nil
}

func (vpc *SVpc) AllowPerformSyncstatus(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) bool {
	return db.IsAdminAllowPerform(userCred, vpc, "syncstatus")
}

// 同步VPC状态
func (vpc *SVpc) PerformSyncstatus(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, input api.VpcSyncstatusInput) (jsonutils.JSONObject, error) {
	return vpc.PerformSync(ctx, userCred, query, input)
}

func (vpc *SVpc) AllowPerformSync(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, data jsonutils.JSONObject) bool {
	return db.IsAdminAllowPerform(userCred, vpc, "sync")
}

func (vpc *SVpc) PerformSync(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, input api.VpcSyncstatusInput) (jsonutils.JSONObject, error) {
	if vpc.IsManaged() {
		return nil, StartResourceSyncStatusTask(ctx, userCred, vpc, "VpcSyncstatusTask", "")
	}
	return nil, httperrors.NewUnsupportOperationError("on-premise vpc cannot sync status")
}

func (self *SVpc) initWire(ctx context.Context, zone *SZone) (*SWire, error) {
	wire := &SWire{
		Bandwidth: 10000,
		Mtu:       1500,
	}
	wire.VpcId = self.Id
	wire.ZoneId = zone.Id
	wire.IsEmulated = true
	wire.Name = fmt.Sprintf("vpc-%s", self.Name)
	wire.SetModelManager(WireManager, wire)
	err := WireManager.TableSpec().Insert(ctx, wire)
	if err != nil {
		return nil, err
	}
	return wire, nil
}

func GetVpcQuotaKeysFromCreateInput(input api.VpcCreateInput) quotas.SDomainRegionalCloudResourceKeys {
	ownerId := &db.SOwnerId{DomainId: input.ProjectDomain}
	var region *SCloudregion
	if len(input.Cloudregion) > 0 {
		region = CloudregionManager.FetchRegionById(input.Cloudregion)
	}
	var provider *SCloudprovider
	if len(input.Cloudprovider) > 0 {
		provider = CloudproviderManager.FetchCloudproviderById(input.Cloudprovider)
	}
	regionKeys := fetchRegionalQuotaKeys(rbacutils.ScopeDomain, ownerId, region, provider)
	keys := quotas.SDomainRegionalCloudResourceKeys{}
	keys.SBaseDomainQuotaKeys = regionKeys.SBaseDomainQuotaKeys
	keys.SRegionalBaseKeys = regionKeys.SRegionalBaseKeys
	keys.SCloudResourceBaseKeys = regionKeys.SCloudResourceBaseKeys
	return keys
}

func (vpc *SVpc) GetQuotaKeys() quotas.SDomainRegionalCloudResourceKeys {
	region, _ := vpc.GetRegion()
	manager := vpc.GetCloudprovider()
	ownerId := vpc.GetOwnerId()
	regionKeys := fetchRegionalQuotaKeys(rbacutils.ScopeDomain, ownerId, region, manager)
	keys := quotas.SDomainRegionalCloudResourceKeys{}
	keys.SBaseDomainQuotaKeys = regionKeys.SBaseDomainQuotaKeys
	keys.SRegionalBaseKeys = regionKeys.SRegionalBaseKeys
	keys.SCloudResourceBaseKeys = regionKeys.SCloudResourceBaseKeys
	return keys
}

func (vpc *SVpc) GetUsages() []db.IUsage {
	if vpc.Deleted {
		return nil
	}
	usage := SInfrasQuota{Vpc: 1}
	keys := vpc.GetQuotaKeys()
	usage.SetKeys(keys)
	return []db.IUsage{
		&usage,
	}
}

func (manager *SVpcManager) totalCount(
	ownerId mcclient.IIdentityProvider,
	scope rbacutils.TRbacScope,
	rangeObjs []db.IStandaloneModel,
	providers []string,
	brands []string,
	cloudEnv string,
) int {
	q := VpcManager.Query()

	if scope != rbacutils.ScopeSystem && ownerId != nil {
		q = q.Equals("domain_id", ownerId.GetProjectDomainId())
	}
	q = CloudProviderFilter(q, q.Field("manager_id"), providers, brands, cloudEnv)
	q = RangeObjectsFilter(q, rangeObjs, q.Field("cloudregion_id"), nil, q.Field("manager_id"), nil, nil)

	cnt, _ := q.CountWithError()

	return cnt
}

func (vpc *SVpc) GetChangeOwnerCandidateDomainIds() []string {
	candidates := [][]string{
		vpc.SManagedResourceBase.GetChangeOwnerCandidateDomainIds(),
	}
	globalVpc, _ := vpc.GetGlobalVpc()
	if globalVpc != nil {
		candidates = append(candidates, db.ISharableChangeOwnerCandidateDomainIds(globalVpc))
	}
	log.Debugf("Candidate: %s", candidates)
	return db.ISharableMergeChangeOwnerCandidateDomainIds(vpc, candidates...)
}

func (vpc *SVpc) GetRequiredSharedDomainIds() []string {
	wires := vpc.GetWires()
	if len(wires) == 0 {
		return vpc.SEnabledStatusInfrasResourceBase.GetRequiredSharedDomainIds()
	}
	requires := make([][]string, len(wires))
	for i := range wires {
		requires[i] = db.ISharableChangeOwnerCandidateDomainIds(&wires[i])
	}
	return db.ISharableMergeShareRequireDomainIds(requires...)
}

func (manager *SVpcManager) ListItemExportKeys(ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	keys stringutils2.SSortedStrings,
) (*sqlchemy.SQuery, error) {
	q, err := manager.SEnabledStatusInfrasResourceBaseManager.ListItemExportKeys(ctx, q, userCred, keys)
	if err != nil {
		return nil, errors.Wrap(err, "SEnabledStatusInfrasResourceBaseManager.ListItemExportKeys")
	}
	if keys.ContainsAny(manager.SCloudregionResourceBaseManager.GetExportKeys()...) {
		q, err = manager.SCloudregionResourceBaseManager.ListItemExportKeys(ctx, q, userCred, keys)
		if err != nil {
			return nil, errors.Wrap(err, "SCloudregionResourceBaseManager.ListItemExportKeys")
		}
	}
	if keys.ContainsAny(manager.SManagedResourceBaseManager.GetExportKeys()...) {
		q, err = manager.SManagedResourceBaseManager.ListItemExportKeys(ctx, q, userCred, keys)
		if err != nil {
			return nil, errors.Wrap(err, "SManagedResourceBaseManager.ListItemExportKeys")
		}
	}
	if keys.ContainsAny(manager.SGlobalVpcResourceBaseManager.GetExportKeys()...) {
		q, err = manager.SGlobalVpcResourceBaseManager.ListItemExportKeys(ctx, q, userCred, keys)
		if err != nil {
			return nil, errors.Wrap(err, "SGlobalVpcResourceBaseManager.ListItemExportKeys")
		}
	}

	if keys.Contains("wire_count") {
		wires := WireManager.Query("vpc_id").SubQuery()
		subq := wires.Query(sqlchemy.COUNT("wire_count"), wires.Field("vpc_id")).GroupBy(wires.Field("vpc_id")).SubQuery()
		q = q.Join(subq, sqlchemy.Equals(q.Field("id"), subq.Field("vpc_id")))
		q = q.AppendField(subq.Field("wire_count"))
	}

	if keys.Contains("network_count") {
		wires := WireManager.Query("id", "vpc_id").SubQuery()
		networks := NetworkManager.Query("wire_id").SubQuery()
		subq := networks.Query(sqlchemy.COUNT("network_count"), wires.Field("vpc_id"))
		subq = subq.Join(wires, sqlchemy.Equals(networks.Field("wire_id"), wires.Field("id")))
		subq = subq.GroupBy(wires.Field("vpc_id"))
		subqQ := subq.SubQuery()
		q = q.Join(subqQ, sqlchemy.Equals(q.Field("id"), subqQ.Field("vpc_id")))
		q = q.AppendField(subqQ.Field("network_count"))
	}

	return q, nil
}

func (vpc *SVpc) PerformPublic(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, input apis.PerformPublicDomainInput) (jsonutils.JSONObject, error) {
	_, err := vpc.SEnabledStatusInfrasResourceBase.PerformPublic(ctx, userCred, query, input)
	if err != nil {
		return nil, errors.Wrap(err, "SEnabledStatusInfrasResourceBase.PerformPublic")
	}
	// perform public for all emulated wires
	wires := vpc.GetWires()
	for i := range wires {
		if wires[i].IsEmulated {
			_, err := wires[i].PerformPublic(ctx, userCred, query, input)
			if err != nil {
				return nil, errors.Wrap(err, "wire.PerformPublic")
			}
		}
	}
	return nil, nil
}

func (vpc *SVpc) PerformPrivate(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, input apis.PerformPrivateInput) (jsonutils.JSONObject, error) {
	// perform private for all emulated wires
	emptyNets := true
	wires := vpc.GetWires()
	for i := range wires {
		if wires[i].DomainId == vpc.DomainId {
			nets, _ := wires[i].getNetworks(nil, rbacutils.ScopeNone)
			for j := range nets {
				if nets[j].DomainId != vpc.DomainId {
					emptyNets = false
					break
				}
			}
			if !emptyNets {
				break
			}
		} else {
			emptyNets = false
			break
		}
	}
	if emptyNets {
		for i := range wires {
			nets, _ := wires[i].getNetworks(nil, rbacutils.ScopeNone)
			netfail := false
			for j := range nets {
				if nets[j].IsPublic && nets[j].GetPublicScope().HigherEqual(rbacutils.ScopeDomain) {
					var err error
					if consts.GetNonDefaultDomainProjects() {
						netinput := apis.PerformPublicProjectInput{}
						netinput.Scope = string(rbacutils.ScopeDomain)
						_, err = nets[j].PerformPublic(ctx, userCred, nil, netinput)
					} else {
						_, err = nets[j].PerformPrivate(ctx, userCred, nil, input)
					}
					if err != nil {
						log.Errorf("nets[j].PerformPublic fail %s", err)
						netfail = true
						break
					}
				}
			}
			if netfail {
				break
			}
			_, err := wires[i].PerformPrivate(ctx, userCred, query, input)
			if err != nil {
				log.Errorf("wires[i].PerformPrivate fail %s", err)
				break
			}
		}
	}
	return vpc.SEnabledStatusInfrasResourceBase.PerformPrivate(ctx, userCred, query, input)
}

func (vpc *SVpc) PerformChangeOwner(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject, input apis.PerformChangeDomainOwnerInput) (jsonutils.JSONObject, error) {
	_, err := vpc.SEnabledStatusInfrasResourceBase.PerformChangeOwner(ctx, userCred, query, input)
	if err != nil {
		return nil, errors.Wrap(err, "SEnabledStatusInfrasResourceBase.PerformChangeOwner")
	}
	wires := vpc.GetWires()
	for i := range wires {
		if wires[i].IsEmulated {
			_, err := wires[i].PerformChangeOwner(ctx, userCred, query, input)
			if err != nil {
				return nil, errors.Wrap(err, "wires[i].PerformChangeOwner")
			}
		}
	}

	return nil, nil
}
