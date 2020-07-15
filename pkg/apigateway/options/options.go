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

package options

import (
	common_options "yunion.io/x/onecloud/pkg/cloudcommon/options"
)

type GatewayOptions struct {
	DefaultRegion string `help:"Use default region while region not specific in api request"`
	CookieDomain  string `help:"specific yunionauth cookie domain" default:""`

	Timeout int `help:"Timeout in seconds, default is 300" default:"300"`

	DisableModuleApiVersion bool `help:"Disable each modules default api version" default:"false"`

	EnableTotp bool `help:"Enable two-factor authentication" default:"true"`

	SsoRedirectUrl     string `help:"SSO idp redirect URL"`
	SsoAuthCallbackUrl string `help:"SSO idp auth callback URL"`
	SsoLinkCallbackUrl string `help:"SSO idp link user callback URL"`

	common_options.CommonOptions `"request_worker_count->default":"32"`
}

var (
	Options *GatewayOptions
)

func OnOptionsChange(oldO, newO interface{}) bool {
	oldOpts := oldO.(*GatewayOptions)
	newOpts := newO.(*GatewayOptions)

	changed := false
	if common_options.OnCommonOptionsChange(&oldOpts.CommonOptions, &newOpts.CommonOptions) {
		changed = true
	}

	return changed
}
