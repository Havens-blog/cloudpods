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

package compute

import (
	"yunion.io/x/onecloud/pkg/apis"
	"yunion.io/x/onecloud/pkg/cloudprovider"
)

const (
	CLOUD_PROVIDER_INIT          = "init"
	CLOUD_PROVIDER_CONNECTED     = "connected"
	CLOUD_PROVIDER_DISCONNECTED  = "disconnected"
	CLOUD_PROVIDER_START_DELETE  = "start_delete"
	CLOUD_PROVIDER_DELETING      = "deleting"
	CLOUD_PROVIDER_DELETED       = "deleted"
	CLOUD_PROVIDER_DELETE_FAILED = "delete_failed"

	CLOUD_PROVIDER_SYNC_STATUS_QUEUING = "queuing"
	CLOUD_PROVIDER_SYNC_STATUS_QUEUED  = "queued"
	CLOUD_PROVIDER_SYNC_STATUS_SYNCING = "syncing"
	CLOUD_PROVIDER_SYNC_STATUS_IDLE    = "idle"
	CLOUD_PROVIDER_SYNC_STATUS_ERROR   = "error"

	CLOUD_PROVIDER_ONECLOUD  = "OneCloud"
	CLOUD_PROVIDER_VMWARE    = "VMware"
	CLOUD_PROVIDER_ALIYUN    = "Aliyun"
	CLOUD_PROVIDER_APSARA    = "Apsara"
	CLOUD_PROVIDER_QCLOUD    = "Qcloud"
	CLOUD_PROVIDER_AZURE     = "Azure"
	CLOUD_PROVIDER_AWS       = "Aws"
	CLOUD_PROVIDER_HUAWEI    = "Huawei"
	CLOUD_PROVIDER_OPENSTACK = "OpenStack"
	CLOUD_PROVIDER_UCLOUD    = "Ucloud"
	CLOUD_PROVIDER_ZSTACK    = "ZStack"
	CLOUD_PROVIDER_GOOGLE    = "Google"
	CLOUD_PROVIDER_CTYUN     = "Ctyun"

	CLOUD_PROVIDER_GENERICS3 = "S3"
	CLOUD_PROVIDER_CEPH      = "Ceph"
	CLOUD_PROVIDER_XSKY      = "Xsky"

	CLOUD_PROVIDER_HEALTH_NORMAL        = "normal"        // 远端处于健康状态
	CLOUD_PROVIDER_HEALTH_INSUFFICIENT  = "insufficient"  // 不足按需资源余额
	CLOUD_PROVIDER_HEALTH_SUSPENDED     = "suspended"     // 远端处于冻结状态
	CLOUD_PROVIDER_HEALTH_ARREARS       = "arrears"       // 远端处于欠费状态
	CLOUD_PROVIDER_HEALTH_UNKNOWN       = "unknown"       // 未知状态，查询失败
	CLOUD_PROVIDER_HEALTH_NO_PERMISSION = "no permission" // 没有权限获取账单信息

	ZSTACK_BRAND_DSTACK     = "DStack"
	ONECLOUD_BRAND_ONECLOUD = "OneCloud"
)

const (
	CLOUD_ACCESS_ENV_AWS_GLOBAL          = CLOUD_PROVIDER_AWS + "-int"
	CLOUD_ACCESS_ENV_AWS_CHINA           = CLOUD_PROVIDER_AWS
	CLOUD_ACCESS_ENV_AZURE_GLOBAL        = CLOUD_PROVIDER_AZURE + "-int"
	CLOUD_ACCESS_ENV_AZURE_GERMAN        = CLOUD_PROVIDER_AZURE + "-de"
	CLOUD_ACCESS_ENV_AZURE_US_GOVERNMENT = CLOUD_PROVIDER_AZURE + "-us-gov"
	CLOUD_ACCESS_ENV_AZURE_CHINA         = CLOUD_PROVIDER_AZURE
	CLOUD_ACCESS_ENV_HUAWEI_GLOBAL       = CLOUD_PROVIDER_HUAWEI + "-int"
	CLOUD_ACCESS_ENV_HUAWEI_CHINA        = CLOUD_PROVIDER_HUAWEI
	CLOUD_ACCESS_ENV_ALIYUN_GLOBAL       = CLOUD_PROVIDER_ALIYUN
	CLOUD_ACCESS_ENV_ALIYUN_FINANCE      = CLOUD_PROVIDER_ALIYUN + "-fin"
	CLOUD_ACCESS_ENV_CTYUN_CHINA         = CLOUD_PROVIDER_CTYUN
)

var (
	CLOUD_PROVIDER_VALID_STATUS        = []string{CLOUD_PROVIDER_CONNECTED}
	CLOUD_PROVIDER_VALID_HEALTH_STATUS = []string{CLOUD_PROVIDER_HEALTH_NORMAL, CLOUD_PROVIDER_HEALTH_NO_PERMISSION}
	PRIVATE_CLOUD_PROVIDERS            = []string{CLOUD_PROVIDER_ZSTACK, CLOUD_PROVIDER_OPENSTACK}

	CLOUD_PROVIDERS = []string{
		CLOUD_PROVIDER_ONECLOUD,
		CLOUD_PROVIDER_VMWARE,
		CLOUD_PROVIDER_ALIYUN,
		CLOUD_PROVIDER_APSARA,
		CLOUD_PROVIDER_QCLOUD,
		CLOUD_PROVIDER_AZURE,
		CLOUD_PROVIDER_AWS,
		CLOUD_PROVIDER_HUAWEI,
		CLOUD_PROVIDER_OPENSTACK,
		CLOUD_PROVIDER_UCLOUD,
		CLOUD_PROVIDER_ZSTACK,
		CLOUD_PROVIDER_GOOGLE,
		CLOUD_PROVIDER_CTYUN,
	}
)

const (
	CLOUD_ENV_PUBLIC_CLOUD  = cloudprovider.CLOUD_ENV_PUBLIC_CLOUD
	CLOUD_ENV_PRIVATE_CLOUD = cloudprovider.CLOUD_ENV_PRIVATE_CLOUD
	CLOUD_ENV_ON_PREMISE    = cloudprovider.CLOUD_ENV_ON_PREMISE

	CLOUD_ENV_PRIVATE_ON_PREMISE = cloudprovider.CLOUD_ENV_PRIVATE_ON_PREMISE
)

const (
	CLOUD_ACCOUNT_SHARE_MODE_ACCOUNT_DOMAIN  = apis.CLOUD_ACCOUNT_SHARE_MODE_ACCOUNT_DOMAIN
	CLOUD_ACCOUNT_SHARE_MODE_SYSTEM          = apis.CLOUD_ACCOUNT_SHARE_MODE_SYSTEM
	CLOUD_ACCOUNT_SHARE_MODE_PROVIDER_DOMAIN = apis.CLOUD_ACCOUNT_SHARE_MODE_PROVIDER_DOMAIN
)

var (
	CLOUD_ACCOUNT_SHARE_MODES = []string{
		CLOUD_ACCOUNT_SHARE_MODE_ACCOUNT_DOMAIN,
		CLOUD_ACCOUNT_SHARE_MODE_SYSTEM,
		CLOUD_ACCOUNT_SHARE_MODE_PROVIDER_DOMAIN,
	}
)
