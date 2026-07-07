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

package tools

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	semver "github.com/Masterminds/semver/v3"
	"github.com/go-playground/validator/v10"

	"github.com/datarobot/cli/internal/log"
)

// createPrereqValidatorOnce initializes the shared validator exactly once.
// validator.New() is expensive (allocates reflection caches) and RegisterValidation /
// RegisterTagNameFunc are not safe to call concurrently, so we use sync.OnceValue
// to guarantee a single, goroutine-safe initialization.
var createPrereqValidatorOnce = sync.OnceValue(func() *validator.Validate {
	v := validator.New()

	// Use the yaml struct tag as the field name in validation errors so that
	// messages reference YAML keys ("minimum-version") rather than Go field
	// names ("MinimumVersion"), matching what users see in versions.yaml.
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		if name := f.Tag.Get("yaml"); name != "" && name != "-" {
			return name
		}

		return f.Name
	})

	_ = v.RegisterValidation("semver", func(fl validator.FieldLevel) bool {
		_, err := semver.NewVersion(fl.Field().String())

		return err == nil
	})

	return v
})

// validatePrerequisite validates a single Prerequisite entry from versions.yaml.
// Violations are returned as human-readable strings and logged as warnings.
func validatePrerequisite(key string, p Prerequisite) []string {
	var errs validator.ValidationErrors

	prereqValidator := createPrereqValidatorOnce()

	if err := prereqValidator.Struct(p); !errors.As(err, &errs) {
		return nil
	}

	violations := make([]string, 0, len(errs))

	for _, fe := range errs {
		violations = append(violations, fieldViolationMsg(key, fe))
	}

	return violations
}

func fieldViolationMsg(key string, fe validator.FieldError) string {
	var msg string

	switch fe.Tag() {
	case "required":
		msg = fmt.Sprintf("versions.yaml [%s]: '%s' is required", key, fe.Field())
	case "semver":
		msg = fmt.Sprintf("versions.yaml [%s]: '%s' %q is not a valid semantic version", key, fe.Field(), fe.Value())
	default:
		msg = fmt.Sprintf("versions.yaml [%s]: '%s' failed validation (%s)", key, fe.Field(), fe.Tag())
	}

	log.Warn(msg)

	return msg
}
