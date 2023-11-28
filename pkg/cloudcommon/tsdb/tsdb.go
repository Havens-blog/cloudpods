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

package tsdb

import (
	"math/rand"

	"yunion.io/x/pkg/errors"

	"yunion.io/x/onecloud/pkg/apis"
	"yunion.io/x/onecloud/pkg/mcclient"
)

type TSDBServiceSource struct {
	Type string
	URLs []string
}

func NewTSDBServiceSource(t string, urls []string) *TSDBServiceSource {
	return &TSDBServiceSource{
		Type: t,
		URLs: urls,
	}
}

func GetDefaultServiceSource(s *mcclient.ClientSession, endpointType string) (*TSDBServiceSource, error) {
	errs := []error{}
	for _, sType := range []string{apis.SERVICE_TYPE_INFLUXDB, apis.SERVICE_TYPE_VICTORIA_METRICS} {
		urls, err := s.GetServiceURLs(sType, endpointType)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "get %s service type %q", endpointType, sType))
		}
		if len(urls) != 0 {
			return NewTSDBServiceSource(sType, urls), nil
		}
	}
	return nil, errors.NewAggregate(errs)
}

func GetDefaultServiceSourceURLs(s *mcclient.ClientSession, endpointType string) ([]string, error) {
	src, err := GetDefaultServiceSource(s, endpointType)
	if err != nil {
		return nil, errors.Wrap(err, "GetDefaultServiceSource")
	}
	if len(src.URLs) == 0 {
		return nil, errors.Errorf("tsdb source %q URLs are empty", src.Type)
	}
	return src.URLs, nil
}

func GetDefaultServiceSourceURL(s *mcclient.ClientSession, endpointType string) (string, error) {
	urls, err := GetDefaultServiceSourceURLs(s, endpointType)
	if err != nil {
		return "", err
	}
	return urls[rand.Intn(len(urls))], nil
}
