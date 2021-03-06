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
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	pltcm "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	plttyp "github.com/ethereum/go-ethereum/core/types"
	pltcli "github.com/ethereum/go-ethereum/ethclient"
	"github.com/palettechain/palette-relayer/config"
	"github.com/palettechain/palette-relayer/db"
	"github.com/palettechain/palette-relayer/log"
	"github.com/palettechain/palette-relayer/utils/palette"
	"github.com/palettechain/palette-relayer/utils/rest"
	"github.com/polynetwork/eth-contracts/go_abi/eccm_abi"
	polysdk "github.com/polynetwork/poly-go-sdk"
	ccm "github.com/polynetwork/poly/native/service/cross_chain_manager/common"
	synccm "github.com/polynetwork/poly/native/service/header_sync/common"
	autils "github.com/polynetwork/poly/native/service/utils"
)

var (
	polyHeaderSyncContract    = autils.HeaderSyncContractAddress.ToHexString()
	polyCrossChainMgrContract = autils.CrossChainManagerContractAddress
)

type pltEpoch struct {
	height uint64
	valset []pltcm.Address
	raw    []byte
}

// PaletteManager 从palette->poly:
// 1.需要使用polysdk记录/获取 poly/native/services/palette(记录来自palette同步的区块头信息) 的同步高度.
// 2.也需要一个palette client记录/获取palette solidity contracts相关内容/接口.
// 3.在向poly中继链提交header和proof时需要poly 中继链上的账户signer 进行签名.
// 4.同步的过程，往往是批量处理，而不是逐个区块进行同步或者提交事件等，所以需要有两个slice记录header和crossTx.
// 5.为保证不落下每个区块我们需要一个存储在中继链headerSync合约上的 currentHeight标志记录当前处理到哪个块了，
// 如果出现分叉等现象还需要一个forceHeight重置区块高度.
// 6.需要一个cross chain manager从palette链上solidity合约获取跨链事件
type PaletteManager struct {
	config *config.ServiceConfig
	db     *db.BoltDB

	restClient       *rest.RestClient
	paletteClient    *pltcli.Client
	paletteLockProxy *eccm_abi.EthCrossChainManager
	polySdk          *polysdk.PolySdk
	polySigner       *polysdk.Account

	currentSyncHeaderHeight,
	currentDepositHeight,
	forceHeight uint64

	lastEpoch,
	curHeader *pltEpoch

	exitChan chan int
}

func NewPaletteManager(
	cfg *config.ServiceConfig,
	startHeight,
	startForceHeight uint64,
	polySdk *polysdk.PolySdk,
	paletteClient *pltcli.Client,
	boltDB *db.BoltDB,
) (*PaletteManager, error) {

	signer, err := cfg.OpenPolyWallet(polySdk)
	if err != nil {
		return nil, err
	}

	if len(cfg.TargetContracts) == 0 {
		return nil, fmt.Errorf("NewETHManager - no target contracts")
	}

	lockAddress := pltcm.HexToAddress(cfg.PaletteConfig.ECCMContractAddress)
	lockContract, err := eccm_abi.NewEthCrossChainManager(lockAddress, paletteClient)
	if err != nil {
		log.Errorf("NewPaletteManager - generate instance of cross chain manager err: %s", err.Error())
		return nil, err
	}

	restCli := rest.NewRestClient()
	palette.Initialize(cfg.PaletteConfig.RestURL, restCli)

	mgr := &PaletteManager{
		config:                  cfg,
		exitChan:                make(chan int),
		currentSyncHeaderHeight: startHeight,
		forceHeight:             startForceHeight,
		restClient:              restCli,
		paletteClient:           paletteClient,
		paletteLockProxy:        lockContract,
		polySdk:                 polySdk,
		polySigner:              signer,
		db:                      boltDB,
	}

	if err := mgr.init(); err != nil {
		log.Errorf("NewPaletteManager - init manager err: %s", err)
		return nil, err
	}

	log.Infof("NewPaletteManager - poly signer address: %s", signer.Address.ToBase58())
	return mgr, nil
}

// init find latest block height on poly chain and valset this value as current height of `palette manager`.
// when an irreversible error occurs in the cross-chain process, we can start over by some fixed block
// height which is lower than the current height.
func (m *PaletteManager) init() error {
	lastEpoch := m.findLastEpochHeight()
	if lastEpoch == 0 {
		return fmt.Errorf("init - the genesis block has not synced!")
	}
	if !m.fetchLastEpoch(lastEpoch) {
		return fmt.Errorf("init - find the genesis header failded")
	}

	curHeight := m.db.GetPaletteHeight()
	if curHeight == 0 {
		curHeight = lastEpoch
	}
	if m.forceHeight > 0 && m.forceHeight < curHeight {
		curHeight = m.forceHeight
	}

	m.currentSyncHeaderHeight = curHeight
	m.currentDepositHeight = curHeight
	log.Infof("PaletteManager init - start height: %d", curHeight)

	return nil
}

// MonitorChain the `paletteManager` needs to traverse all of blocks and events on the palette chain,
// and record the relationship of block height and block content which contains block header and event logs.
// and these data should be synced to `headerSync` and `crossChainManager` contracts located on poly chain.
func (m *PaletteManager) MonitorChain() {
	ticker := time.NewTicker(config.PLT_MONITOR_INTERVAL)
	for {
		select {
		case <-ticker.C:
			height, err := palette.GetNodeHeight()
			if err != nil {
				log.Infof("PaletteManager MonitorChain - cannot get node height, err: %s", err)
				continue
			}

			for m.currentSyncHeaderHeight < height {
				if m.handleNewBlock(m.currentSyncHeaderHeight) {
					_ = m.db.UpdatePaletteHeight(m.currentSyncHeaderHeight)
					m.currentSyncHeaderHeight++
					log.Infof("PaletteManager MonitorChain - current height %d, palette height is %d",
						m.currentSyncHeaderHeight, height)
				} else {
					time.Sleep(1 * time.Second)
				}
			}

		case <-m.exitChan:
			return
		}
	}
}

func (m *PaletteManager) MonitorDeposit() {
	ticker := time.NewTicker(config.PLT_MONITOR_INTERVAL)
	for {
		select {
		case <-ticker.C:
			for m.currentDepositHeight < m.currentSyncHeaderHeight {
				_ = m.handleDepositEvents(m.currentDepositHeight)
				m.currentDepositHeight++
			}
		case <-m.exitChan:
			return
		}
	}
}

func (m *PaletteManager) CheckDeposit() {
	ticker := time.NewTicker(config.PLT_MONITOR_INTERVAL)
	for {
		select {
		case <-ticker.C:
			_ = m.checkLockEvents()
		case <-m.exitChan:
			return
		}
	}
}

// findLastEpochHeight get current block height which recorded on `crossChainManager` contract located on poly chain.
func (m *PaletteManager) findLastEpochHeight() uint64 {
	key := m.formatStorageKey(synccm.CONSENSUS_PEER_BLOCK_HEIGHT, nil)
	result, _ := m.polySdk.GetStorage(polyHeaderSyncContract, key)
	return bytesToUint64(result)
}

// handleNewBlock retry if handle block header failed. if handle events failed, just ignore.
func (m *PaletteManager) handleNewBlock(height uint64) bool {
	if m.checkEpochHeight(height) {
		if !m.fetchBlockHeader(height) {
			log.Errorf("PaletteManager handleNewBlock - fetchBlockHeader on height :%d failed", height)
			return false
		}

		if m.isEpoch() && !m.commitHeader() {
			log.Errorf("PaletteManager handleNewBlock - commitHeader on height :%d failed", height)
			return false
		}
	}

	if !m.fetchLockEvents(height) {
		log.Errorf("PaletteManager handleNewBlock - fetchLockEvents on height :%d failed", height)
	}
	return true
}

// fetchBlockHeader get block header from palette chain, append header
// in cache if it's not exist in poly chain.
func (m *PaletteManager) fetchBlockHeader(height uint64) bool {
	if m.curHeader != nil && m.curHeader.height == height {
		return true
	}

	// get validators of current block
	hdr, err := m.paletteClient.HeaderByNumber(context.Background(), uint64ToBig(height))
	if err != nil {
		log.Errorf("PaletteManager fetchBlockHeader - GetNodeHeader on height :%d failed", height)
		return false
	}

	// compare header
	raw, err := hdr.MarshalJSON()
	if err != nil {
		log.Errorf("PaletteManager fetchBlockHeader - marshal current block header err: %s", err)
		return false
	}
	if m.curHeader != nil && bytes.Equal(raw, m.curHeader.raw) {
		return true
	}

	// get validators
	extra, err := plttyp.ExtractIstanbulExtra(hdr)
	if err != nil {
		log.Errorf("PaletteManager fetchBlockHeader - extract istanbul extra err: %s", err)
		return false
	}

	m.curHeader = &pltEpoch{
		height: height,
		raw:    raw,
		valset: extra.Validators,
	}

	return true
}

// get validators of last pltEpoch
func (m *PaletteManager) fetchLastEpoch(height uint64) bool {
	key := m.formatStorageKey(
		synccm.CONSENSUS_PEER,
		nil,
	)
	raw, err := m.polySdk.GetStorage(polyHeaderSyncContract, key)
	if err != nil {
		log.Errorf("PaletteManager fetchLastEpoch - get storage err: %s", err)
		return false
	}

	vals, err := bytes2Valset(raw)
	if err != nil {
		log.Errorf("PaletteManager fetchLastEpoch - deserialize poly valset err: %s", err)
		return false
	}

	m.lastEpoch = &pltEpoch{
		height: height,
		raw:    raw,
		valset: vals,
	}

	return true
}

func (m *PaletteManager) commitHeader() bool {
	tx, err := m.polySdk.Native.Hs.SyncBlockHeader(
		m.sideChainID(),
		m.polySigner.Address,
		[][]byte{m.curHeader.raw},
		m.polySigner,
	)
	if err != nil {
		log.Errorf("PaletteManager commitHeader - sync block header err: %s", err)
		return false
	}

	// waiting for transaction confirmed on poly chain, and the landmark event is that
	// current block height on poly chain is bigger than tx's height.
	var h uint32
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		if h == 0 {
			h, _ = m.polySdk.GetBlockHeightByTxHash(tx.ToHexString())
		} else {
			curr, _ := m.polySdk.GetCurrentBlockHeight()
			if curr > h {
				m.lastEpoch.height = m.curHeader.height
				m.lastEpoch.raw = m.curHeader.raw
				m.lastEpoch.valset = m.curHeader.valset
				break
			}
		}
	}

	log.Infof("PaletteManager commitHeader - send (palette transaction %s, palette header height %d, valset size %d) "+
		"to poly chain and confirmed on poly height %d", tx.ToHexString(), m.curHeader.height, len(m.curHeader.valset), h)

	return true
}

func (m *PaletteManager) isEpoch() bool {
	s1 := m.curHeader.valset
	s2 := m.lastEpoch.valset

	if len(s1) != len(s2) {
		return true
	}

	sortAddrList(s1)
	sortAddrList(s2)

	for i := 0; i < len(s1); i++ {
		if s1[i] != s2[i] {
			return true
		}
	}

	return false
}

// checkEpochHeight return true if height is bigger than last pltEpoch height
func (m *PaletteManager) checkEpochHeight(height uint64) bool {
	if height <= m.lastEpoch.height {
		return false
	}
	return true
}

// fetchLockEvents get cross chain events from lock proxy contract which located on palette chain.
// filter events which has incorrect contract address or already exist in poly chain, and cache these
// events data in `retry` bucket of blot database.
func (m *PaletteManager) fetchLockEvents(height uint64) bool {
	opt := &bind.FilterOpts{
		Start:   height,
		End:     &height,
		Context: context.Background(),
	}
	iter, err := m.paletteLockProxy.FilterCrossChainEvent(opt, nil)
	if err != nil {
		debug("PaletteManager fetchLockEvents - FilterCrossChainEvent error :%s", err.Error())
		return false
	}
	if iter == nil {
		debug("PaletteManager fetchLockEvents - no event iter found on FilterCrossChainEvent")
		return false
	}

	for iter.Next() {
		evt := iter.Event
		addr := evt.ProxyOrAssetContract
		if !m.config.TargetContracts.CheckContract(addr, "outbound", evt.ToChainId) {
			continue
		}

		param := recoverMakeTxParams(evt.Rawdata)
		if !m.checkCrossChainEvent(param) {
			continue
		}

		_, sink := serializeCrossTransfer(evt, height)
		if err := m.db.PutRetry(sink.Bytes()); err != nil {
			log.Errorf("PaletteManager fetchLockEvents - m.db.PutRetry error: %s", err)
		} else {
			log.Infof("PaletteManager fetchLockEvents -  height: %d", height)
		}
	}
	return true
}

// handleDepositEvents
func (m *PaletteManager) handleDepositEvents(refHeight uint64) error {
	retryList, err := m.db.GetAllRetry()
	if err != nil {
		return fmt.Errorf("handleDepositEvents - m.db.GetAllRetry error: %s", err)
	}

	for _, v := range retryList {
		crossTx, err := deserializeCrossTransfer(v)
		if err != nil {
			log.Errorf("PaletteManager handleDepositEvents - retry.Deserialization error: %s", err)
			continue
		}

		// poly do not allow to verify header with validators in old epoch,
		// we need to waiting for some blocks to fetch the latest block header and proof.
		// and quorum only add/del single node in one epoch.
		// safeHeight used for avoid chain fork, just need 1 block.
		distance := m.safeBlockDistance()
		if refHeight-distance <= crossTx.height {
			log.Infof("PaletteManager handleDepositEvents - ignore tx %s, refHeight %d - distance %d <= crossTx height %d",
				crossTx.txIndex, refHeight, distance, crossTx.height)
			continue
		}
		safeHeight := refHeight - 1

		// get proof from palette chain
		proof, hdr, err := m.getProof(crossTx, safeHeight)
		if err != nil {
			log.Errorf("PaletteManager handleDepositEvents - get proof error :%s\n", err.Error())
			continue
		}

		// commit proof to poly chain success
		txHash, err := m.commitProof(uint32(safeHeight), proof, crossTx.value, crossTx.txId, hdr)
		if err != nil {
			if strings.Contains(err.Error(), "chooseUtxos, current utxo is not enough") {
				log.Infof("PaletteManager handleDepositEvents - invokeNativeContract error: %s", err)
			} else if strings.Contains(err.Error(), "tx already done") {
				log.Infof("PaletteManager handleDepositEvents - plt_tx %s already on poly", txIdHex(crossTx.txId))
				if err := m.db.DeleteRetry(v); err != nil {
					log.Errorf("PaletteManager handleDepositEvents - deleteRetry error: %s", err)
				}
			} else {
				log.Errorf("PaletteManager handleDepositEvents - invoke NativeContract for block %d eth_tx %s: %s, err %s",
					safeHeight, txIdHex(crossTx.txId), err)
			}
			continue
		}

		// process cache
		if err := m.db.PutCheck(txHash, v); err != nil {
			log.Errorf("PaletteManager handleDepositEvents - this.db.PutCheck error: %s", err)
		}
		if err := m.db.DeleteRetry(v); err != nil {
			log.Errorf("PaletteManager handleDepositEvents - this.db.PutCheck error: %s", err)
		}
		log.Infof("PaletteManager handleDepositEvents - %s", crossTx.txIndex)
	}
	return nil
}

func (m *PaletteManager) getProof(e *CrossTransfer, height uint64) (proof []byte, hdr []byte, err error) {
	// decode events
	keyBytes, err := getMappingKey(e.txIndex)
	if err != nil {
		return
	}
	proofKey := hexutil.Encode(keyBytes)
	heightHex := uint64ToHex(height)

	// get proof from palette chain
	proof, err = palette.GetProof(m.eccdContract(), proofKey, heightHex)
	if err != nil {
		return
	}

	var block *plttyp.Block
	block, err = m.paletteClient.BlockByNumber(context.Background(), uint64ToBig(height))
	if err != nil {
		return
	}

	hdr, err = block.Header().MarshalJSON()
	return
}

func (m *PaletteManager) commitProof(
	height uint32,
	proof []byte,
	txData []byte,
	txhash []byte,
	hdr []byte,
) (string, error) {

	debug("PaletteManager - commit proof, height: %d, proof: %s, txData: %s, txhash: %s",
		height, string(proof), hex.EncodeToString(txData), hex.EncodeToString(txhash))

	sideChainId := m.sideChainID()
	relayAddr := pltcm.Hex2Bytes(m.polySigner.Address.ToHexString())
	tx, err := m.polySdk.Native.Ccm.ImportOuterTransfer(
		sideChainId,
		txData,
		height,
		proof,
		relayAddr,
		hdr,
		m.polySigner,
	)
	if err != nil {
		return "", err
	}

	debug("PaletteManager - commitProof debug:"+
		" hash %s, header %s, txData %s, proof %s, height %d",
		pltcm.BytesToHash(txhash).Hex(),
		hexutil.Encode(hdr),
		hexutil.Encode(txData),
		hexutil.Encode(proof),
		height,
	)

	log.Infof("PaletteManager commitProof - send transaction to poly chain: "+
		"( poly_txhash: %s, plt_txhash: %s, height: %d )",
		tx.ToHexString(), pltcm.BytesToHash(txhash).String(), height)

	return tx.ToHexString(), nil
}

func (m *PaletteManager) checkLockEvents() error {
	checkMap, err := m.db.GetAllCheck()
	if err != nil {
		return fmt.Errorf("checkLockEvents - m.db.GetAllCheck error: %s", err)
	}

	for txhash, v := range checkMap {
		event, err := m.polySdk.GetSmartContractEvent(txhash)
		if err != nil {
			log.Errorf("PaletteManager checkLockEvents - m.aliaSdk.GetSmartContractEvent error: %s", err)
			continue
		}
		if event == nil {
			continue
		}

		if event.State != 1 {
			log.Errorf("PaletteManager checkLockEvents - state of poly tx %s is failed", txhash)
			if err := m.db.PutRetry(v); err != nil {
				log.Errorf("PaletteManager checkLockEvents - m.db.PutRetry error:%s", err)
			}
		}

		if err = m.db.DeleteCheck(txhash); err != nil {
			log.Errorf("PaletteManager checkLockEvents - m.db.DeleteRetry error:%s", err)
		}

		log.Infof("PaletteManager checkLockEvents - state of poly tx %s is success!", txhash)
	}
	return nil
}

// checkCrossChainEvent return false if cross chain event from palette lock proxy contract
// is already existed in poly chain.
func (m *PaletteManager) checkCrossChainEvent(param *ccm.MakeTxParam) bool {
	key := m.formatStorageKey(ccm.DONE_TX, param.CrossChainID)
	raw, _ := m.polySdk.GetStorage(polyCrossChainMgrContract.ToHexString(), key)
	if len(raw) != 0 {
		log.Debugf("PaletteManager fetchLockEvents - ccid %s (tx_hash: %s) already on poly",
			hex.EncodeToString(param.CrossChainID), pltcm.BytesToHash(param.TxHash))
		return false
	}
	return true
}

func (m *PaletteManager) formatStorageKey(prefix string, content []byte) []byte {
	key := []byte(prefix)
	chainID := m.sideChainID()
	key = append(key, autils.GetUint64Bytes(chainID)...)
	if content != nil {
		key = append(key, content...)
	}
	return key
}

func (m *PaletteManager) eccdContract() string {
	return m.config.PaletteConfig.ECCDContractAddress
}

func (m *PaletteManager) eccmContract() string {
	return m.config.PaletteConfig.ECCMContractAddress
}

// usually add/del single node need 4 blocks, and relayer should waiting for at least 1 block to avoid palette chain fork.
const defaultDistance = 6

func (m *PaletteManager) safeBlockDistance() uint64 {
	if m.config.PaletteConfig.BlockConfig < defaultDistance {
		return defaultDistance
	} else {
		return m.config.PaletteConfig.BlockConfig
	}
}

func (m *PaletteManager) sideChainID() uint64 {
	return m.config.PaletteConfig.SideChainId
}
