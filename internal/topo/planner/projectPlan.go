// Copyright 2021-2025 EMQ Technologies Co., Ltd.
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

package planner

import (
	"strconv"

	"github.com/lf-edge/ekuiper/v2/pkg/ast"
)

type ProjectPlan struct {
	baseLogicalPlan
	isAggregate      bool
	allWildcard      bool
	sendMeta         bool
	sendNil          bool
	fields           ast.Fields
	fieldLen         int
	colNames         [][]string
	exceptNames      []string
	wildcardEmitters map[string]bool
	aliasFields      ast.Fields
	exprFields       ast.Fields
	enableLimit      bool
	limitCount       int
}

func (p ProjectPlan) Init() *ProjectPlan {
	p.allWildcard = false
	p.wildcardEmitters = make(map[string]bool)
	for _, field := range p.fields {
		if field.AName != "" {
			p.aliasFields = append(p.aliasFields, field)
		} else {
			switch ft := field.Expr.(type) {
			case *ast.Wildcard:
				p.allWildcard = true
				p.exceptNames = ft.Except
				for _, replace := range ft.Replace {
					p.aliasFields = append(p.aliasFields, replace)
				}
			case *ast.FieldRef:
				if ft.Name == "*" {
					p.wildcardEmitters[string(ft.StreamName)] = true
				} else {
					if !field.Invisible {
						p.colNames = append(p.colNames, []string{ft.Name, string(ft.StreamName)})
					}
				}
			default:
				p.exprFields = append(p.exprFields, field)
			}
		}
	}
	p.baseLogicalPlan.self = &p
	p.baseLogicalPlan.setPlanType(PROJECT)
	return &p
}

func (p *ProjectPlan) BuildExplainInfo() {
	info := ""
	if p.fields != nil && len(p.fields) != 0 {
		info += "Fields:[ "
		for i, field := range p.fields {
			if field.Expr != nil {
				info += field.Expr.String()
				if i != len(p.fields)-1 {
					info += ", "
				}
			}
		}
		info += " ]"
	}
	if p.enableLimit {
		info += ", Limit:" + strconv.Itoa(p.limitCount)
	}
	p.baseLogicalPlan.ExplainInfo.Info = info
}

func (p *ProjectPlan) PruneColumns(fields []ast.Expr) error {
	f := getFields(p.fields)
	return p.baseLogicalPlan.PruneColumns(append(fields, f...))
}
