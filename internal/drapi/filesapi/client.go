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

package filesapi

import (
	"io"
)

type Client interface {
	CreateCatalog() (*CatalogResp, error)
	CreateStage(catalogID string) (*StageResp, error)
	UploadToStage(catalogID, stageID, name string, size int64, body io.Reader) error
	ApplyStage(catalogID, stageID, overwrite string) (*ApplyStageResp, error)
	UploadFromZipNew(name string, size int64, body io.Reader) (*FromFileResp, error)
	UploadFromZipExisting(catalogID, name, overwrite string, size int64, body io.Reader) (*FromFileResp, error)
	PollStatus(statusID string) (*StatusResp, error)
	AllFiles(catalogID, versionID string) (map[string]FileMeta, error)
	DownloadFile(catalogID, versionID, path string, w io.Writer) (string, int64, error)
	DeleteFiles(catalogID string, paths []string) (*DeleteFilesResp, error)
}

func New() Client {
	return &httpClient{}
}

type httpClient struct{}
