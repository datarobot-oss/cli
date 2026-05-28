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

package wapi

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	drvalidate "github.com/datarobot/cli/internal/validate"
	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/go-playground/validator/v10"
)

var (
	validateOnce sync.Once
	validateV    *validator.Validate
)

func getValidator() *validator.Validate {
	validateOnce.Do(func() {
		v := validator.New()
		_ = drvalidate.RegisterDRTags(v)
		validateV = v
	})

	return validateV
}

func validateConfig(cfg Config) error {
	if err := getValidator().Struct(cfg); err != nil {
		return formatStructValidation(err)
	}

	if ptrSet(cfg.LastSyncedVersionID) && !ptrSet(cfg.CatalogID) {
		return errors.New("catalogId is required when lastSyncedVersionId is set")
	}

	return nil
}

func ptrSet(p *string) bool {
	return p != nil && *p != ""
}

func validateManifest(m Manifest) error {
	if err := getValidator().Struct(m); err != nil {
		return formatStructValidation(err)
	}

	if err := validateManifestSyncState(m); err != nil {
		return err
	}

	for path := range m.Files {
		if err := fileops.SafeRelPath(path); err != nil {
			return fmt.Errorf("files[%q]: %w", path, err)
		}
	}

	return nil
}

func validateManifestSyncState(m Manifest) error {
	hasVersion := m.SyncedVersionID != nil && *m.SyncedVersionID != ""
	hasSyncedAt := m.SyncedAt != nil && !m.SyncedAt.IsZero()

	if hasVersion && !hasSyncedAt {
		return errors.New("syncedAt is required when syncedVersionId is set")
	}

	if hasSyncedAt && !hasVersion {
		return errors.New("syncedVersionId is required when syncedAt is set")
	}

	return nil
}

func validateInitOptions(opts InitOptions) error {
	if err := getValidator().Struct(opts); err != nil {
		return formatStructValidation(err)
	}

	if opts.LastSyncedVersionID != "" && opts.CatalogID == "" {
		return errors.New("catalogId is required when lastSyncedVersionId is set")
	}

	return nil
}

func formatStructValidation(err error) error {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return err
	}

	msgs := make([]string, 0, len(verrs))
	for _, fe := range verrs {
		msgs = append(msgs, formatFieldError(fe))
	}

	return errors.New(strings.Join(msgs, "; "))
}

func formatFieldError(fe validator.FieldError) string {
	field := jsonFieldName(fe)

	switch fe.Tag() {
	case "required":
		return field + " is required"
	case "eq":
		return fmt.Sprintf("%s must equal %s", field, fe.Param())
	case "gte":
		return fmt.Sprintf("%s must be >= %s", field, fe.Param())
	case "len":
		return fmt.Sprintf("%s must be %s characters", field, fe.Param())
	case drvalidate.TagDRSHA256Hex:
		return field + " must be a 64-character lowercase SHA-256 hex string"
	case drvalidate.TagDRID:
		return field + " must be a non-empty identifier without path separators"
	case drvalidate.TagDRNonemptyPtr:
		return field + " must not be empty when set"
	default:
		return fmt.Sprintf("%s failed validation (%s)", field, fe.Tag())
	}
}

// jsonFieldNames maps struct field names to JSON keys for user-facing errors.
var jsonFieldNames = map[string]string{
	"ArtifactID":          "artifactId",
	"CatalogID":           "catalogId",
	"LastSyncedVersionID": "lastSyncedVersionId",
	"CreatedAt":           "createdAt",
	"CLIVersion":          "cliVersion",
	"Version":             "version",
	"SyncedAt":            "syncedAt",
	"SyncedVersionID":     "syncedVersionId",
	"Hash":                "hash",
	"Size":                "size",
}

func jsonFieldName(fe validator.FieldError) string {
	if name, ok := jsonFieldNames[fe.StructField()]; ok {
		return name
	}

	return fe.Field()
}
