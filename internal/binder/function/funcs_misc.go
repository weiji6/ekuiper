// Copyright 2022-2025 EMQ Technologies Co., Ltd.
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

package function

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	b64 "encoding/base64"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lf-edge/ekuiper/contract/v2/api"

	"github.com/lf-edge/ekuiper/v2/internal/conf"
	"github.com/lf-edge/ekuiper/v2/internal/keyedstate"
	"github.com/lf-edge/ekuiper/v2/internal/topo/context"
	"github.com/lf-edge/ekuiper/v2/pkg/ast"
	"github.com/lf-edge/ekuiper/v2/pkg/cast"
	"github.com/lf-edge/ekuiper/v2/pkg/props"
	"github.com/lf-edge/ekuiper/v2/pkg/timex"
)

func registerMiscFunc() {
	gob.Register(&ringqueue{})
	builtins["bypass"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			return args[0], true
		},
		val: func(ctx api.FunctionContext, args []ast.Expr) error {
			return nil
		},
		check: func(args []interface{}) (interface{}, bool) {
			return args, false
		},
	}
	builtins["props"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			key, ok := args[0].(string)
			if !ok {
				return fmt.Errorf("invalid input %v: must be property name of string type", args[0]), false
			}
			return props.SC.Get(key)
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["cast"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			value := args[0]
			newType := args[1]
			return cast.ToType(value, newType)
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(2, len(args)); err != nil {
				return err
			}
			a := args[1]
			if ast.IsNumericArg(a) || ast.IsTimeArg(a) || ast.IsBooleanArg(a) {
				return ProduceErrInfo(0, "string")
			}
			if av, ok := a.(*ast.StringLiteral); ok {
				if !(av.Val == "bigint" || av.Val == "float" || av.Val == "string" || av.Val == "boolean" || av.Val == "datetime" || av.Val == "bytea") {
					return fmt.Errorf("Expect one of following value for the 2nd parameter: bigint, float, string, boolean, datetime, bytea.")
				}
			}
			return nil
		},
		check: returnNilIfHasAnyNil,
	}
	builtins["convert_tz"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			arg0, err := cast.InterfaceToTime(args[0], "")
			if err != nil {
				return err, false
			}
			arg1 := cast.ToStringAlways(args[1])
			loc, err := time.LoadLocation(arg1)
			if err != nil {
				return err, false
			}
			return arg0.In(loc), true
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(2, len(args)); err != nil {
				return err
			}
			if ast.IsNumericArg(args[0]) || ast.IsStringArg(args[0]) || ast.IsBooleanArg(args[0]) {
				return ProduceErrInfo(0, "datetime")
			}
			if ast.IsNumericArg(args[1]) || ast.IsTimeArg(args[1]) || ast.IsBooleanArg(args[1]) {
				return ProduceErrInfo(1, "string")
			}
			return nil
		},
		check: returnNilIfHasAnyNil,
	}
	builtins["to_seconds"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			t, err := cast.InterfaceToTime(args[0], "")
			if err != nil {
				return err, false
			}
			return t.Unix(), true
		},
		val:   ValidateOneArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["to_json"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			rr, err := json.Marshal(args[0])
			if err != nil {
				return fmt.Errorf("fail to convert %v to json", args[0]), false
			}
			return string(rr), true
		},
		val:   ValidateOneArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["parse_json"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			if args[0] == nil || args[0] == "null" {
				return nil, true
			}
			text, err := cast.ToString(args[0], cast.CONVERT_SAMEKIND)
			if err != nil {
				return fmt.Errorf("fail to convert %v to string", args[0]), false
			}
			var data interface{}
			err = json.Unmarshal(cast.StringToBytes(text), &data)
			if err != nil {
				return fmt.Errorf("fail to parse json: %v", err), false
			}
			return data, true
		},
		val: ValidateOneStrArg,
	}
	builtins["chr"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			switch v := args[0].(type) {
			case int:
				return rune(v), true
			case int64:
				return rune(v), true
			case float64:
				return rune(v), true
			case string:
				if len(v) > 1 {
					return fmt.Errorf("Parameter length cannot larger than 1."), false
				}
				r := []rune(v)
				return r[0], true
			default:
				return fmt.Errorf("Only bigint, float and string type can be convert to char type."), false
			}
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(1, len(args)); err != nil {
				return err
			}
			if ast.IsFloatArg(args[0]) || ast.IsTimeArg(args[0]) || ast.IsBooleanArg(args[0]) {
				return ProduceErrInfo(0, "int")
			}
			return nil
		},
		check: returnNilIfHasAnyNil,
	}
	builtins["encode"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			if v, ok := args[1].(string); ok {
				if strings.EqualFold(v, "base64") {
					if v1, ok1 := args[0].(string); ok1 {
						return b64.StdEncoding.EncodeToString([]byte(v1)), true
					} else {
						return fmt.Errorf("Only string type can be encoded."), false
					}
				} else {
					return fmt.Errorf("Only base64 encoding is supported."), false
				}
			}
			return nil, false
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(2, len(args)); err != nil {
				return err
			}

			if ast.IsNumericArg(args[0]) || ast.IsTimeArg(args[0]) || ast.IsBooleanArg(args[0]) {
				return ProduceErrInfo(0, "string")
			}

			a := args[1]
			if !ast.IsStringArg(a) {
				return ProduceErrInfo(1, "string")
			}
			if av, ok := a.(*ast.StringLiteral); ok {
				if av.Val != "base64" {
					return fmt.Errorf("Only base64 is supported for the 2nd parameter.")
				}
			}
			return nil
		},
		check: returnNilIfHasAnyNil,
	}
	builtins["decode"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			if v, ok := args[1].(string); ok {
				if strings.EqualFold(v, "base64") {
					if v1, ok1 := args[0].(string); ok1 {
						r, e := b64.StdEncoding.DecodeString(v1)
						if e != nil {
							return fmt.Errorf("fail to decode base64 string: %v", e), false
						}
						return r, true
					} else {
						return fmt.Errorf("Only string type can be decoded."), false
					}
				} else {
					return fmt.Errorf("Only base64 decoding is supported."), false
				}
			}
			return nil, false
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(2, len(args)); err != nil {
				return err
			}

			if ast.IsNumericArg(args[0]) || ast.IsTimeArg(args[0]) || ast.IsBooleanArg(args[0]) {
				return ProduceErrInfo(0, "string")
			}

			a := args[1]
			if !ast.IsStringArg(a) {
				return ProduceErrInfo(1, "string")
			}
			if av, ok := a.(*ast.StringLiteral); ok {
				if av.Val != "base64" {
					return fmt.Errorf("Only base64 is supported for the 2nd parameter.")
				}
			}
			return nil
		},
		check: returnNilIfHasAnyNil,
	}
	builtins["trunc"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			var v0 float64
			v0, err := cast.ToFloat64(args[0], cast.CONVERT_SAMEKIND)
			if err != nil {
				return err, false
			}
			switch v2 := args[1].(type) {
			case int:
				return toFixed(v0, v2), true
			case int64:
				return toFixed(v0, int(v2)), true
			default:
				return fmt.Errorf("The 2nd parameter must be int value."), false
			}
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(2, len(args)); err != nil {
				return err
			}

			if ast.IsTimeArg(args[0]) || ast.IsBooleanArg(args[0]) || ast.IsStringArg(args[0]) {
				return ProduceErrInfo(0, "number - float or int")
			}

			if ast.IsFloatArg(args[1]) || ast.IsTimeArg(args[1]) || ast.IsBooleanArg(args[1]) || ast.IsStringArg(args[1]) {
				return ProduceErrInfo(1, "int")
			}
			return nil
		},
		check: returnNilIfHasAnyNil,
	}
	builtins["md5"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			arg0 := cast.ToStringAlways(args[0])
			h := md5.New()
			_, err := io.WriteString(h, arg0)
			if err != nil {
				return err, false
			}
			return fmt.Sprintf("%x", h.Sum(nil)), true
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["sha1"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			arg0 := cast.ToStringAlways(args[0])
			h := sha1.New()
			_, err := io.WriteString(h, arg0)
			if err != nil {
				return err, false
			}
			return fmt.Sprintf("%x", h.Sum(nil)), true
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["sha256"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			arg0 := cast.ToStringAlways(args[0])
			h := sha256.New()
			_, err := io.WriteString(h, arg0)
			if err != nil {
				return err, false
			}
			return fmt.Sprintf("%x", h.Sum(nil)), true
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["sha384"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			arg0 := cast.ToStringAlways(args[0])
			h := sha512.New384()
			_, err := io.WriteString(h, arg0)
			if err != nil {
				return err, false
			}
			return fmt.Sprintf("%x", h.Sum(nil)), true
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["sha512"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			arg0 := cast.ToStringAlways(args[0])
			h := sha512.New()
			_, err := io.WriteString(h, arg0)
			if err != nil {
				return err, false
			}
			return fmt.Sprintf("%x", h.Sum(nil)), true
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["crc32"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			arg0 := cast.ToStringAlways(args[0])
			return fmt.Sprintf("%x", crc32.ChecksumIEEE([]byte(arg0))), true
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
	builtinStatfulFuncs["compress"] = func() api.Function {
		conf.Log.Infof("initializing compress function")
		return &compressFunc{}
	}
	builtinStatfulFuncs["decompress"] = func() api.Function {
		conf.Log.Infof("initializing decompress function")
		return &decompressFunc{}
	}
	builtins["isnull"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			if args[0] == nil {
				return true, true
			} else {
				v := reflect.ValueOf(args[0])
				switch v.Kind() {
				case reflect.Slice, reflect.Map:
					return v.IsNil(), true
				default:
					return false, true
				}
			}
		},
		val: ValidateOneArg,
	}
	builtins["coalesce"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			for _, arg := range args {
				if arg != nil {
					return arg, true
				}
			}
			return nil, true
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if len(args) == 0 {
				return fmt.Errorf("The arguments should be at least one.")
			}
			return nil
		},
	}
	builtins["newuuid"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			if newUUID, err := uuid.NewUUID(); err != nil {
				return err, false
			} else {
				return newUUID.String(), true
			}
		},
		val: ValidateNoArg,
	}
	builtins["tstamp"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			return timex.GetNowInMilli(), true
		},
		val: ValidateNoArg,
	}
	builtins["mqtt"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			if v, ok := args[0].(string); ok {
				return v, true
			}
			return nil, false
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(1, len(args)); err != nil {
				return err
			}
			if ast.IsIntegerArg(args[0]) || ast.IsTimeArg(args[0]) || ast.IsBooleanArg(args[0]) || ast.IsStringArg(args[0]) || ast.IsFloatArg(args[0]) {
				return ProduceErrInfo(0, "meta reference")
			}
			if p, ok := args[0].(*ast.MetaRef); ok {
				name := strings.ToLower(p.Name)
				if name != "topic" && name != "messageid" {
					return fmt.Errorf("Parameter of mqtt function can be only topic or messageid.")
				}
			}
			return nil
		},
		check: returnNilIfHasAnyNil,
	}
	builtins["rule_id"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			return ctx.GetRuleId(), true
		},
		val: ValidateNoArg,
	}
	builtins["rule_start"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			return ctx.Value(context.RuleStartKey), true
		},
		val: ValidateNoArg,
	}
	builtins["meta"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			return args[0], true
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(1, len(args)); err != nil {
				return err
			}
			if _, ok := args[0].(*ast.MetaRef); ok {
				return nil
			}
			expr := args[0]
			for {
				if be, ok := expr.(*ast.BinaryExpr); ok {
					if _, ok := be.LHS.(*ast.MetaRef); ok && be.OP == ast.ARROW {
						return nil
					}
					expr = be.LHS
				} else {
					break
				}
			}
			return ProduceErrInfo(0, "meta reference")
		},
	}
	builtins["cardinality"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			val := reflect.ValueOf(args[0])
			if val.Kind() == reflect.Slice {
				return val.Len(), true
			}
			return 0, true
		},
		val:   ValidateOneArg,
		check: return0IfHasAnyNil,
	}
	builtins["json_path_query"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			result, err := jsonCall(ctx, args)
			if err != nil {
				return err, false
			}
			return result, true
		},
		val: ValidateJsonFunc,
	}
	builtins["json_path_query_first"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			result, err := jsonCall(ctx, args)
			if err != nil {
				return err, false
			}
			if arr, ok := result.([]interface{}); ok {
				return arr[0], true
			} else {
				return fmt.Errorf("query result (%v) is not an array", result), false
			}
		},
		val: ValidateJsonFunc,
	}
	builtins["json_path_exists"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			result, err := jsonCall(ctx, args)
			if err != nil {
				return false, true
			}
			if result == nil {
				return false, true
			}
			e := true
			switch reflect.TypeOf(result).Kind() {
			case reflect.Slice, reflect.Array:
				e = reflect.ValueOf(result).Len() > 0
			default:
				e = result != nil
			}
			return e, true
		},
		val: ValidateJsonFunc,
	}
	builtins["window_trigger"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec:  nil, // directly return in the valuer
		val:   ValidateNoArg,
	}
	builtins["window_start"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec:  nil, // directly return in the valuer
		val:   ValidateNoArg,
	}
	builtins["window_end"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec:  nil, // directly return in the valuer
		val:   ValidateNoArg,
	}
	builtins["event_time"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec:  nil, // directly return in the valuer
		val:   ValidateNoArg,
	}

	builtins["delay"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			d, err := cast.ToInt(args[0], cast.CONVERT_SAMEKIND)
			if err != nil {
				return err, false
			}
			time.Sleep(time.Duration(d) * time.Millisecond)
			return args[1], true
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(2, len(args)); err != nil {
				return err
			}
			if ast.IsStringArg(args[0]) || ast.IsTimeArg(args[0]) || ast.IsBooleanArg(args[0]) {
				return ProduceErrInfo(0, "number - float or int")
			}
			return nil
		},
		check: returnNilIfHasAnyNil,
	}
	builtins["get_keyed_state"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			if len(args) != 3 {
				return fmt.Errorf("the args must be two or three"), false
			}
			key, ok := args[0].(string)
			if !ok {
				return fmt.Errorf("key %v is not a string", args[0]), false
			}

			value, err := keyedstate.GetKeyedState(key)
			if err != nil {
				return args[2], true
			}

			return cast.ToType(value, args[1])
		},
		val: func(_ api.FunctionContext, args []ast.Expr) error {
			if err := ValidateLen(3, len(args)); err != nil {
				return err
			}
			a := args[1]
			if ast.IsNumericArg(a) || ast.IsTimeArg(a) || ast.IsBooleanArg(a) {
				return ProduceErrInfo(0, "string")
			}
			if av, ok := a.(*ast.StringLiteral); ok {
				if !(av.Val == "bigint" || av.Val == "float" || av.Val == "string" || av.Val == "boolean" || av.Val == "datetime") {
					return fmt.Errorf("expect one of following value for the 2nd parameter: bigint, float, string, boolean, datetime")
				}
			}
			return nil
		},
	}
	builtins["hex2dec"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			hex, ok := args[0].(string)
			if !ok {
				return fmt.Errorf("invalid input type: %v please input hex string", args[0]), false
			}
			hex = strings.TrimPrefix(hex, "0x")
			dec, err := strconv.ParseInt(hex, 16, 64)
			if err != nil {
				return fmt.Errorf("invalid hexadecimal value: %v", hex), false
			}
			return dec, true
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
	builtins["dec2hex"] = builtinFunc{
		fType: ast.FuncTypeScalar,
		exec: func(ctx api.FunctionContext, args []interface{}) (interface{}, bool) {
			dec, err := cast.ToInt(args[0], cast.STRICT)
			if err != nil {
				return err, false
			}
			hex := "0x" + strconv.FormatInt(int64(dec), 16)
			return hex, true
		},
		val:   ValidateOneStrArg,
		check: returnNilIfHasAnyNil,
	}
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func jsonCall(ctx api.StreamContext, args []interface{}) (interface{}, error) {
	jp, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("invalid jsonPath, must be a string but got %v", args[1])
	}
	return ctx.ParseJsonPath(jp, args[0])
}

// page Rotate storage for in memory cache
// Not thread safe!
type ringqueue struct {
	Data []any
	H    int
	T    int
	L    int
	Size int
}

func newRingqueue(size int) *ringqueue {
	return &ringqueue{
		Data: make([]interface{}, size),
		H:    0, // When deleting, head++, if tail == head, it is empty
		T:    0, // When append, tail++, if tail== head, it is full
		Size: size,
	}
}

// fill item will fill the queue with item value
func (p *ringqueue) fill(item interface{}) {
	for {
		if !p.append(item) {
			return
		}
	}
}

// append item if list is not full and return true; otherwise return false
func (p *ringqueue) append(item interface{}) bool {
	if p.L == p.Size { // full
		return false
	}
	p.Data[p.T] = item
	p.T++
	if p.T == p.Size {
		p.T = 0
	}
	p.L++
	return true
}

// fetch get the first item in the cache and remove
func (p *ringqueue) fetch() (interface{}, bool) {
	if p.L == 0 {
		return nil, false
	}
	result := p.Data[p.H]
	p.H++
	if p.H == p.Size {
		p.H = 0
	}
	p.L--
	return result, true
}

// peek get the first item in the cache but keep it
func (p *ringqueue) peek() (interface{}, bool) {
	if p.L == 0 {
		return nil, false
	}
	result := p.Data[p.H]
	return result, true
}

func (p *ringqueue) isFull() bool {
	return p.L == p.Size
}
