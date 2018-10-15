// Copyright 2018 The Cockroach Authors.
//
// Licensed as a CockroachDB Enterprise file under the Cockroach Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/cockroachdb/cockroach/blob/master/licenses/CCL.txt

package cliccl

import (
	"fmt"
	"time"

	"github.com/cockroachdb/cockroach/pkg/ccl/baseccl"
	"github.com/cockroachdb/cockroach/pkg/ccl/storageccl/engineccl/enginepbccl"
	"github.com/cockroachdb/cockroach/pkg/storage/engine"
)

// Defines CCL-specific debug commands, adds the encryption flag to debug commands in
// `pkg/cli/debug.go`, and registers a callback to generate encryption options.

var encryptionStatusOpts struct {
	activeStoreIDOnly bool
}

// fillEncryptionOptionsForStore fills the RocksDBConfig fields
// based on the --enterprise-encryption flag value.
func fillEncryptionOptionsForStore(cfg *engine.RocksDBConfig) error {
	opts, err := baseccl.EncryptionOptionsForStore(cfg.Dir, storeEncryptionSpecs)
	if err != nil {
		return err
	}

	if opts != nil {
		cfg.ExtraOptions = opts
		cfg.UseFileRegistry = true
	}
	return nil
}

type keyInfoByAge []*enginepbccl.KeyInfo

func (ki keyInfoByAge) Len() int           { return len(ki) }
func (ki keyInfoByAge) Swap(i, j int)      { ki[i], ki[j] = ki[j], ki[i] }
func (ki keyInfoByAge) Less(i, j int) bool { return ki[i].CreationTime < ki[j].CreationTime }

// JSONTime is a json-marshalable time.Time.
type JSONTime time.Time

// MarshalJSON marshals time.Time into json.
func (t JSONTime) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", time.Time(t).String())), nil
}

// PrettyDataKey is the final json-exportable struct for a data key.
type PrettyDataKey struct {
	ID      string
	Active  bool `json:",omitempty"`
	Exposed bool `json:",omitempty"`
	Created JSONTime
	Files   []string `json:",omitempty"`
}

// PrettyStoreKey is the final json-exportable struct for a store key.
type PrettyStoreKey struct {
	ID       string
	Active   bool `json:",omitempty"`
	Type     string
	Created  JSONTime
	Source   string
	Files    []string        `json:",omitempty"`
	DataKeys []PrettyDataKey `json:",omitempty"`
}
