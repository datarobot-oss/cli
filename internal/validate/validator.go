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

package validate

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

const maxDRIDLen = 256

const (
	// TagDRID is a custom validator tag.
	//
	// It enforces a non-empty identifier that is safe to embed in filesystem
	// paths: max 256 chars, no path separators ('/' or '\\'), and no ".."
	// sequences.
	TagDRID = "dr_id"

	// TagDRNonemptyPtr is a custom validator tag.
	//
	// It rejects the common "JSON empty string where null was intended" case:
	// a non-nil *string must not point to "".
	TagDRNonemptyPtr = "dr_nonempty_ptr"

	// TagDRSHA256Hex is a custom validator tag.
	//
	// It requires exactly 64 lowercase hexadecimal digits, with no "0x" prefix,
	// matching encoding/hex.EncodeToString output for SHA-256 hashes.
	TagDRSHA256Hex = "dr_sha256hex"
)

// RegisterDRTags registers DataRobot CLI custom tags on the provided validator.
//
// Callers typically ignore the returned error because validator.RegisterValidation
// only fails on invalid input (which would be a programmer error).
func RegisterDRTags(v *validator.Validate) error {
	if err := v.RegisterValidation(TagDRID, validateDRID); err != nil {
		return err
	}

	if err := v.RegisterValidation(TagDRNonemptyPtr, validateDRNonemptyPtr); err != nil {
		return err
	}

	if err := v.RegisterValidation(TagDRSHA256Hex, validateDRSHA256Hex); err != nil {
		return err
	}

	return nil
}

func validateDRID(fl validator.FieldLevel) bool {
	switch v := fl.Field().Interface().(type) {
	case string:
		return IsValidDRID(v)
	case *string:
		if v == nil {
			return true
		}

		return IsValidDRID(*v)
	default:
		return false
	}
}

func validateDRNonemptyPtr(fl validator.FieldLevel) bool {
	field := fl.Field()

	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return true
		}

		elem := field.Elem()
		if elem.Kind() != reflect.String {
			return false
		}

		return elem.Len() > 0
	}

	if field.Kind() == reflect.String {
		return field.Len() > 0
	}

	return false
}

func validateDRSHA256Hex(fl validator.FieldLevel) bool {
	s, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}

	return IsSHA256Hex(s)
}

// IsValidDRID reports whether s is a non-empty identifier that is safe to embed
// in a filesystem path segment.
//
// This is intended for IDs that may be used under a project directory (e.g.
// `.wapi/.checkouts/<id>/...`) and therefore must not contain separators or
// traversal sequences.
func IsValidDRID(s string) bool {
	if s == "" || len(s) > maxDRIDLen {
		return false
	}

	if strings.ContainsAny(s, `/\`) {
		return false
	}

	if strings.Contains(s, "..") {
		return false
	}

	return true
}

// IsSHA256Hex reports whether s is a SHA-256 hash encoded as lowercase hex,
// exactly 64 characters, with no "0x" prefix.
func IsSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}

	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' {
			continue
		}

		return false
	}

	return true
}
