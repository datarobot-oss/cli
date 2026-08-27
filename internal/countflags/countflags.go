// Copyright 2026 DataRobot, Inc. and its affiliates.
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

// Package countflags provides pflag.Value constructors for count-like int
// flags (--limit, --offset, --tail, --concurrency) that reject out-of-range
// values at parse time, so invalid input dies in flag parsing with cobra's
// standard "invalid argument" message instead of surfacing from deep inside
// a command's RunE. Commands bind their own int and keep reading it directly;
// Flags().GetInt keeps working because the value reports type "int".
//
// The registration pattern mirrors cmd/internal/pollflags.
package countflags

import (
	"errors"
	"strconv"

	"github.com/spf13/pflag"
)

// boundedIntValue is an int flag value that rejects anything Set parses to
// below min. min is inclusive: PositiveInt registers 1, NonNegativeInt 0.
type boundedIntValue struct {
	p   *int
	min int
	msg string
}

// PositiveInt returns an int flag value with value as the default, for count
// flags that must be at least 1. A page size of 0 would fetch nothing and
// read as an empty result rather than an error, so zero is rejected too.
func PositiveInt(p *int, value int) pflag.Value {
	*p = value

	return &boundedIntValue{p: p, min: 1, msg: "must be a positive integer"}
}

// NonNegativeInt returns an int flag value with value as the default, for
// count flags where 0 is meaningful ("from the start", "no limit") but
// negative values are not.
func NonNegativeInt(p *int, value int) pflag.Value {
	*p = value

	return &boundedIntValue{p: p, min: 0, msg: "must be a non-negative integer"}
}

// Set parses s with the same base-0, 64-bit semantics as pflag's built-in
// int flag, so the only behavior change versus IntVar is the bound check.
func (v *boundedIntValue) Set(s string) error {
	parsed, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return err
	}

	if parsed < int64(v.min) {
		return errors.New(v.msg)
	}

	*v.p = int(parsed)

	return nil
}

func (v *boundedIntValue) String() string {
	return strconv.Itoa(*v.p)
}

// Type reports "int" so pflag's GetInt conversion path is unchanged.
func (v *boundedIntValue) Type() string {
	return "int"
}
