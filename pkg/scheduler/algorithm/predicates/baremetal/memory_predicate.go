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

package baremetal

import (
	"context"

	"yunion.io/x/onecloud/pkg/scheduler/algorithm/predicates"
	"yunion.io/x/onecloud/pkg/scheduler/core"
)

type MemoryPredicate struct {
	BasePredicate
}

func (p *MemoryPredicate) Name() string {
	return "baremetal_memory"
}

func (p *MemoryPredicate) Clone() core.FitPredicate {
	return &MemoryPredicate{}
}

func (p *MemoryPredicate) Execute(ctx context.Context, u *core.Unit, c core.Candidater) (bool, []core.PredicateFailureReason, error) {
	h := predicates.NewPredicateHelper(p, u, c)
	d := u.SchedData()

	useRsvd := h.UseReserved()
	getter := c.Getter()
	freeMemSize := getter.FreeMemorySize(useRsvd)
	reqMemSize := int64(d.Memory)
	if freeMemSize < reqMemSize {
		totalMemSize := getter.TotalMemorySize(useRsvd)
		h.AppendInsufficientResourceError(reqMemSize, totalMemSize, freeMemSize)
		h.SetCapacity(0)
	} else {
		if reqMemSize/freeMemSize != 1 {
			h.Exclude2("memory", freeMemSize, reqMemSize)
		} else {
			h.SetCapacity(1)
		}
	}

	return h.GetResult()
}
