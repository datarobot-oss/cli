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

package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

// newStreamHasher returns a SHA-256 hasher for computing the hash of bytes
// as they are streamed to their destination, without a separate read pass.
// Both uploaders use this so the hash recorded in the manifest is of the
// bytes that actually crossed the wire, not the bytes hashed during planning.
// A file rewritten between plan and upload must leave BASE describing what
// the server received, and only a hash of the streamed bytes can guarantee
// that.
func newStreamHasher() hash.Hash {
	return sha256.New()
}

// streamedEntry finalizes a hasher that observed the streamed bytes into a
// FileEntry carrying the hex-encoded SHA-256 and the byte count. The size
// comes from the caller — f.Stat on the already-open handle for the stage
// path, io.Copy's return for the zip path — so it always describes the same
// bytes the hasher observed, never the Phase-2 planned size.
func streamedEntry(h hash.Hash, size int64) FileEntry {
	return FileEntry{
		Hash: hex.EncodeToString(h.Sum(nil)),
		Size: size,
	}
}
