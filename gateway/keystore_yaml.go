// Copyright 2022 Stichting ThingsIX Foundation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/brocaar/lorawan"
	"github.com/ethereum/go-ethereum/crypto"
	"gopkg.in/yaml.v3"
)

type GatewayYamlFileStore struct {
	Path     string
	gateways []*Gateway
}

func LoadGatewayYamlFileStore(path string) (*GatewayYamlFileStore, error) {
	store := &GatewayYamlFileStore{
		Path: path,
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

type gatewayYAML struct {
	LocalID    lorawan.EUI64 `yaml:"local_id"`
	PrivateKey string        `yaml:"private_key"`
}

func (store *GatewayYamlFileStore) Gateways() []*Gateway {
	return store.gateways
}

func (store *GatewayYamlFileStore) GatewayByThingsIxID(id [32]byte) (*Gateway, error) {
	for _, gw := range store.gateways {
		if bytes.Equal(gw.CompressedPublicKeyBytes, id[:]) {
			return gw, nil
		}
	}
	return nil, ErrNotFound
}

func (store *GatewayYamlFileStore) GatewayByLocalID(id lorawan.EUI64) (*Gateway, error) {
	for _, gw := range store.gateways {
		if gw.LocalGatewayID == id {
			return gw, nil
		}
	}
	return nil, ErrNotFound
}

func (store *GatewayYamlFileStore) GatewayByLocalIDBytes(id []byte) (*Gateway, error) {
	if len(id) == 8 {
		var gid lorawan.EUI64
		copy(gid[:], id)
		return store.GatewayByLocalID(gid)
	}
	return nil, ErrInvalidGatewayID
}

func (store *GatewayYamlFileStore) GatewayByNetworkID(id lorawan.EUI64) (*Gateway, error) {
	for _, gw := range store.gateways {
		if gw.NetworkGatewayID == id {
			return gw, nil
		}
	}
	return nil, ErrNotFound
}

func (store *GatewayYamlFileStore) GatewayByNetworkIDBytes(id []byte) (*Gateway, error) {
	if len(id) == 8 {
		var gid lorawan.EUI64
		copy(gid[:], id)
		return store.GatewayByNetworkID(gid)
	}
	return nil, ErrInvalidGatewayID
}

func (store *GatewayYamlFileStore) AddGateway(localID lorawan.EUI64, key *ecdsa.PrivateKey) error {
	_, err := store.GatewayByLocalID(localID)
	switch {
	case errors.Is(err, ErrNotFound):
		break
	case err == nil:
		return ErrAlreadyExists
	default:
		return err
	}

	// serialize gateway details
	encoded, err := yaml.Marshal([]gatewayYAML{{
		LocalID:    localID,
		PrivateKey: hex.EncodeToString(crypto.FromECDSA(key)),
	}})
	if err != nil {
		return fmt.Errorf("unable to encode gateway store: %w", err)
	}

	// append gateway to store
	f, err := os.OpenFile(store.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(encoded); err != nil {
		return err
	}
	return store.load()
}

func (store *GatewayYamlFileStore) load() error {
	unknownGatewayListRaw, err := os.ReadFile(store.Path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return ErrStoreNotExists
		}
		if len(unknownGatewayListRaw) == 0 {
			return nil
		}
	}

	var gateways []gatewayYAML
	if err := yaml.Unmarshal(unknownGatewayListRaw, &gateways); err != nil {
		return fmt.Errorf("unable to load gateway store: %w", err)
	}

	loaded := make([]*Gateway, len(gateways))
	for i, gw := range gateways {
		key, err := crypto.HexToECDSA(gw.PrivateKey)
		if err != nil {
			return fmt.Errorf("could not decode private key from gateway store")
		}
		if loaded[i], err = NewGateway(gw.LocalID, key); err != nil {
			return fmt.Errorf("unable to load gateway from store: %w", err)
		}
	}

	store.gateways = loaded

	return nil
}