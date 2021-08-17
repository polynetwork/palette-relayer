package manager

import (
	"encoding/binary"
	"testing"

	pltcm "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

func TestLittleEndian(t *testing.T) {

	var (
		result = make([]byte, 8)
		bz     = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	)

	copy(result, bz)
	value := binary.LittleEndian.Uint64(result)
	t.Log(value)
}

// TestPLTGetChainIDFromPolyChain get paletteClient identity
func TestPLTGetChainIDFromPolyChain(t *testing.T) {
	chainID, err := testPolySdk.GetNetworkId()
	assert.NoError(t, err)

	t.Logf("registed network id %d", chainID)
}

// TestHandlerNewBlock sync block header and fetch deposit events,
// header should be expect hex string, and header extra validators should
// contain 8 addresses
func TestPLTHandlerBlockHeader(t *testing.T) {
	debugAddAndDelValidator := true

	blockHeightList := []uint64{4197}
	if debugAddAndDelValidator {
		//blockHeightList = []uint64{
		//	5114, // 8 validator
		//	5115, // 8 validator
		//	5116, // 9 validator
		//	5144, // 9 validator
		//	5145, // 9 validator
		//	5146, // 8 validator
		//}
		blockHeightList = []uint64{
			6568, // 8 validators
			6569, // 9 validators
			6570, // 9 validators
			6571, // 10 validators
			6598, // 10 validators
			6599, // 9 validators
			6604, // 8 validators
		}
	}

	for _, height := range blockHeightList {
		testPLTMgr.fetchBlockHeader(height)
		assert.True(t, len(testPLTMgr.curHeader.valset) > 0)

		enc := testPLTMgr.curHeader.raw
		data := hexutil.Encode(enc)
		header := new(types.Header)
		hash := header.Hash()
		err := header.UnmarshalJSON(enc)
		assert.NoError(t, err)

		t.Logf("block %d hash %s", height, hash.Hex())
		t.Logf("header %s", data)
		t.Log("")

		extra, err := types.ExtractIstanbulExtra(header)
		assert.NoError(t, err)

		validators := extra.Validators
		sortAddrList(validators)
		for _, validator := range validators {
			t.Logf("validator %s", validator.Hex())
		}
		t.Log("")

		committers := make([]pltcm.Address, 0)
		for _, seal := range extra.CommittedSeal {
			addr, err := ecrecoverCommitter(header, seal)
			assert.NoError(t, err)
			committers = append(committers, addr)
		}
		sortAddrList(committers)
		for _, committer := range committers {
			t.Logf("committer %s", committer.Hex())
		}
		t.Log("")

		proposer, err := ecrecoverProposer(header, extra)
		assert.NoError(t, err)

		t.Logf("proposer %s", proposer.Hex())
		t.Logf("validators number %d, committed seal number %d",
			len(extra.Validators), len(extra.CommittedSeal))
		t.Log("=====================================================")
	}
}

// TestPLTFetchBlockEvent fetch block event and put into blot db,
// and `retry` bucket length should be more than 0.
func TestPLTFetchBlockEvent(t *testing.T) {
	var height uint64 = 4197

	succeed := testPLTMgr.fetchLockEvents(height)
	assert.True(t, succeed)

	retrys, err := testPLTMgr.db.GetAllRetry()
	assert.NoError(t, err)
	t.Logf("retry length %d", len(retrys))
	assert.True(t, len(retrys) > 0)
}

func TestPLTCommitHeader(t *testing.T) {
	var height uint64 = 5146

	assert.True(t, testPLTMgr.fetchBlockHeader(height))
	assert.True(t, len(testPLTMgr.curHeader.valset) > 0)

	assert.True(t, testPLTMgr.commitHeader())
	assert.Equal(t, height, testPLTMgr.lastEpoch.height)
}

// TestPLTFindLatestHeight get synced header height
func TestPLTFindLatestHeight(t *testing.T) {
	height := testPLTMgr.findLastEpochHeight()
	t.Logf("poly chain synced to block %d", height)

	assert.True(t, testPLTMgr.fetchLastEpoch(height))
	for _, val := range testPLTMgr.lastEpoch.valset {
		t.Logf("pltEpoch %d validator %s", height, val.Hex())
	}
	t.Logf("validators number %d", len(testPLTMgr.lastEpoch.valset))
}

func TestPLTHandleNewBlocks(t *testing.T) {
	var blockStart, blockEnd uint64 = 6567, 6569

	for i := blockStart; i <= blockEnd; i++ {
		testPLTMgr.handleNewBlock(i)
	}
}

func TestPLTBucketCrossTx(t *testing.T) {
	retryList, err := testPLTMgr.db.GetAllRetry()
	assert.NoError(t, err)

	if len(retryList) == 0 {
		return
	}

	for i, v := range retryList {
		t.Logf("retry%d raw: %s", i, hexutil.Encode(v))
	}
}

// get `raw` from bolt bucket
func TestPLTCommitProof(t *testing.T) {
	var (
		height uint64 = 14420
		raw           = "0x023033200043f645f9be7bba122c2e1322fcacb042a2bb5a4b66dd2b0b3a482e7b212ae8c62000000000000000000000000000000000000000000000000000000000000000032069968925c79a08f4f9bd08cb1361db48dea29b431d57736831c0239728ec4b83140000000000000000000000000000000000000103650000000000000014000000000000000000000000000000000000010306756e6c6f636b4a140000000000000000000000000000000000000103145593b2b8dc63d0ed68aa8f885707b2dc5787e391000064a7b3b6e00d00000000000000000000000000000000000000000000000065000000f135000000000000"
	)

	dec, err := hexutil.Decode(raw)
	assert.NoError(t, err)
	crossTx, err := deserializeCrossTransfer(dec)

	t.Logf("transfer: \r\n txIndex:%s,\r\n height:%d,\r\n toChainID:%d,\r\n txId:%s,\r\n value:%s,",
		crossTx.txIndex,
		crossTx.height,
		crossTx.toChain,
		hexutil.Encode(crossTx.txId),
		hexutil.Encode(crossTx.value),
	)

	proof, hdr, err := testPLTMgr.getProof(crossTx, height)
	assert.NoError(t, err)

	txHash, err := testPLTMgr.commitProof(
		uint32(height),
		proof,
		crossTx.value,
		crossTx.txId,
		hdr,
	)
	assert.NoError(t, err)

	t.Logf("proof tx hash %s", txHash)
}
