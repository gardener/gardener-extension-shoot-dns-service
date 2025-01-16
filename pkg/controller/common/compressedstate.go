// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/andybalholm/brotli"
)

type compressedEntriesState struct {
	CompressedState []byte `json:"compressedState"`
}

// CompressEntriesState compresses the entries state data.
func CompressEntriesState(state []byte) ([]byte, error) {
	if len(state) == 0 || string(state) == "{}" {
		return nil, nil
	}

	var stateCompressed bytes.Buffer
	writer := brotli.NewWriter(&stateCompressed)
	defer writer.Close()

	if _, err := writer.Write(state); err != nil {
		return nil, fmt.Errorf("failed writing entries state data for compression: %w", err)
	}

	// Close ensures any unwritten data is flushed. Without this, the `stateCompressed`
	// buffer would not contain any data. Hence, we have to call it explicitly here after writing, in addition to the
	// 'defer' call above.
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed closing the brotli writer after compressing the entries state data: %w", err)
	}

	return json.Marshal(&compressedEntriesState{CompressedState: stateCompressed.Bytes()})
}

// LooksLikeCompressedEntriesState checks if the given state data has the string compressedState in the first 20 bytes.
func LooksLikeCompressedEntriesState(state []byte) bool {
	if len(state) < len("compressedState") {
		return false
	}

	return bytes.Contains(state[:min(20, len(state))], []byte("compressedState"))
}

// DecompressEntriesState decompresses the entries state data.
func DecompressEntriesState(stateCompressed []byte) ([]byte, error) {
	if len(stateCompressed) == 0 {
		return nil, nil
	}

	var entriesState compressedEntriesState
	if err := json.Unmarshal(stateCompressed, &entriesState); err != nil {
		return nil, fmt.Errorf("failed unmarshalling JSON to compressed entries state structure: %w", err)
	}

	reader := brotli.NewReader(bytes.NewReader(entriesState.CompressedState))
	var state bytes.Buffer
	if _, err := state.ReadFrom(reader); err != nil {
		return nil, fmt.Errorf("failed reading machine state data for decompression: %w", err)
	}

	return state.Bytes(), nil
}
