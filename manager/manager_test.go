/*
* Copyright (C) 2020 The poly network Authors
* This file is part of The poly network library.
*
* The poly network is free software: you can redistribute it and/or modify
* it under the terms of the GNU Lesser General Public License as published by
* the Free Software Foundation, either version 3 of the License, or
* (at your option) any later version.
*
* The poly network is distributed in the hope that it will be useful,
* but WITHOUT ANY WARRANTY; without even the implied warranty of
* MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
* GNU Lesser General Public License for more details.
* You should have received a copy of the GNU Lesser General Public License
* along with The poly network . If not, see <http://www.gnu.org/licenses/>.
 */
package manager

import (
	"fmt"
	"os"
	"path"
	"testing"

	pltcli "github.com/ethereum/go-ethereum/ethclient"
	"github.com/palettechain/palette-relayer/config"
	"github.com/palettechain/palette-relayer/db"
	sdk "github.com/polynetwork/poly-go-sdk"
)

var (
	testConfigPath            = fullPath("config_test.json")
	testPLTForceHeight uint64 = 10000
	testPolySdk        *sdk.PolySdk
	testPLTMgr         *PaletteManager
	testPolyMgr        *PolyManager
)

func TestMain(m *testing.M) {
	srvConfig := config.NewServiceConfig(testConfigPath)

	// create poly sdk
	testPolySdk = sdk.NewPolySdk()
	testPolySdk.NewRpcClient().SetAddress(srvConfig.PolyConfig.RestURL)
	hdr, err := testPolySdk.GetHeaderByHeight(0)
	if err != nil {
		panic(err)
	}
	testPolySdk.SetChainId(hdr.ChainID)

	// create palette sdk
	paletteSDK, err := pltcli.Dial(srvConfig.PaletteConfig.RestURL)
	if err != nil {
		panic(fmt.Sprintf("startServer - cannot dial sync node, err: %s", err))
	}

	// create boltDB
	dbpath := srvConfig.BoltDBPath()
	boltDB, err := db.NewBoltDB(dbpath)
	if err != nil {
		panic(fmt.Sprintf("db.NewWaitingDB error:%s", err))
	}

	// create palette manager
	if mgr, err := NewPaletteManager(
		srvConfig,
		0,
		testPLTForceHeight,
		testPolySdk,
		paletteSDK,
		boltDB,
	); err != nil {
		panic(fmt.Sprintf("create plt manager err:%s", err))
	} else {
		testPLTMgr = mgr
	}

	// create poly manager
	if mgr, err := NewPolyManager(
		srvConfig,
		0,
		testPolySdk,
		paletteSDK,
		boltDB,
	); err != nil {
		panic(fmt.Sprintf("create poly manager err:%s", err))
	} else {
		testPolyMgr = mgr
	}

	os.Exit(m.Run())
}

func fullPath(p string) string {
	if path.IsAbs(p) {
		return p
	}
	return path.Join("../build/", p)
}
