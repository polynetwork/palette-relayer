package manager

import (
	"math/rand"
	"strconv"
	"strings"
	"testing"

	pltcm "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	nvcm "github.com/ethereum/go-ethereum/contracts/native/common"
	"github.com/ethereum/go-ethereum/contracts/native/plt"
	"github.com/ethereum/go-ethereum/contracts/native/utils"
	"github.com/polynetwork/poly/common"
	polycm "github.com/polynetwork/poly/common"
	"github.com/polynetwork/poly/core/signature"
	polytypes "github.com/polynetwork/poly/core/types"
	"github.com/polynetwork/poly/merkle"
	crscm "github.com/polynetwork/poly/native/service/cross_chain_manager/common"
	"github.com/stretchr/testify/assert"
)

func TestGetRouterNumber(t *testing.T) {
	var (
		routineNo int64 = 64
		cnt             = 10
	)
	for i := 0; i < cnt; i++ {
		no := strconv.FormatInt(rand.Int63n(routineNo), 10)
		t.Logf("routine num %s", no)
	}
}

func TestPolyFindLastEpochHeight(t *testing.T) {
	height := testPolyMgr.findLastEpochHeight()
	t.Logf("poly last pltEpoch height %d", height)
}

func TestPolyGenesisBookKeepers(t *testing.T) {
	hdr, err := testPolyMgr.polySdk.GetHeaderByHeight(1)
	assert.NoError(t, err)

	for _, v := range hdr.Bookkeepers {
		addr := polytypes.AddressFromPubKey(v)
		t.Logf("address %s", addr.ToBase58())
	}
}

func TestPolyGetLastEpochPubKeyBytes(t *testing.T) {
	expect := []string{
		"AaodCegA3EWhwd5hRdcKASGnJCPd3RJ3A5",
		"AUd2CBoLZkRN2NwKbCZN2CXEbaFS8Y2jso",
		"AdfCn64T8ayXw76bwBhTXGQawQKFKZWxqB",
		"ALYo97aXPxB5WcfKohBAkH4QXFUHYHpgEH",
	}

	inExpect := func(addr polycm.Address) bool {
		for _, expect := range expect {
			if strings.ToLower(expect) == strings.ToLower(addr.ToBase58()) {
				return true
			}
		}
		return false
	}

	raw, err := testPolyMgr.eccd.GetCurEpochConPubKeyBytes(nil)
	t.Logf("")
	assert.NoError(t, err)
	keepers := DeserializeKeepers(raw)
	assert.True(t, len(keepers) > 1)
	for _, keeper := range keepers {
		inExpect(keeper)
		//assert.True(t, inExpect(keeper))
		t.Logf("keeper: %s", keeper.ToBase58())
	}
}

func TestPolyGetHeader(t *testing.T) {
	var height uint32 = 1
	header, err := testPolyMgr.polySdk.GetHeaderByHeight(height)
	assert.NoError(t, err)

	t.Logf("block %d header %v", height, header)
}

func TestGetMerkleProof(t *testing.T) {
	var (
		height           uint32 = 1
		anchorHeightList        = []uint32{1}
		proofList               = make([]string, len(anchorHeightList))
	)

	for i, anchorHeight := range anchorHeightList {
		proof, err := testPolyMgr.polySdk.GetMerkleProof(height, anchorHeight)
		assert.NoError(t, err)
		proofList[i] = proof.AuditPath
	}
}

func TestPolyCurrentHeight(t *testing.T) {
	height, err := testPolyMgr.polySdk.GetCurrentBlockHeight()
	assert.NoError(t, err)

	t.Logf("poly current height %d", height)
}

func TestPolySelectSender(t *testing.T) {
	sender := testPolyMgr.selectSender()
	assert.True(t, sender != nil)
}

func TestVerifyProof(t *testing.T) {
	var (
		auditPath  = "0xef20698231e1792b6ce63cbae4d91dc3916456497484952a5a3d1f63ca0d9fd27ea76900000000000000200000000000000000000000000000000000000000000000000000000000000000204061945d816b2447da1f783eefbaebae1d9bbff2a63e0ca70614725082033aa2140000000000000000000000000000000000000103690000000000000014000000000000000000000000000000000000010306756e6c6f636b4a140000000000000000000000000000000000000103145593b2b8dc63d0ed68aa8f885707b2dc5787e391000064a7b3b6e00d000000000000000000000000000000000000000000000000"
		headerData = "0x00000000db056dd1000000006cb80beaedecc3c1f88e70b2f393fdfc3948733c1447638b2f97aea0dbaa1eb60000000000000000000000000000000000000000000000000000000000000000f605278b9fe4bea01f09d4f8d1803ae0f2ec5e9a6bd85b18924ed4f8f905b25e1b52a770c45cb9e1c71808fdf22c5b6151c04c9cec23497478f4d787061e176dff98ea5fff7f0700e6009e4247a15cb2fd11017b226c6561646572223a322c227672665f76616c7565223a224249354e43342b3666716d4a645274626a2f424278364d656b56394b423266647537397a6d67536d6443706467682f3031746b7772794a694c4556535769462f5278596338704b35554558766e3170654274332f54696f3d222c227672665f70726f6f66223a225864482f55434e4c31334244557359766335526650566c445054496f7737663151355277574543553357484c754a454b4930322f426d4b564c3271727266354b6e4475467765334a366734755578657a615142742b673d3d222c226c6173745f636f6e6669675f626c6f636b5f6e756d223a3437303536352c226e65775f636861696e5f636f6e666967223a6e756c6c7d0000000000000000000000000000000000000000"
		//rawProof   = "0x20f2f930e55e78253098082d3b7ccfe391ca80e7265351ecd2fa3cfa84299d8dd500134b7dfdbd00552b604c514fc165b5dcf3332a73f6371b37cf3693d76a0589010065dab635968f5285b1349971deeb3e84fb89265b99e151cf6eea12bae83de189003645968852f948246d9bedf44c7528b1c0b8ddd4188fb271e1f29188f24891da00dffc03db3ff3090d388e05286a33fb3b12077974a1e6d8eab12163e10238a548"
		//rawAnchor  = "0x00000000db056dd100000000f2f930e55e78253098082d3b7ccfe391ca80e7265351ecd2fa3cfa84299d8dd50000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000071d32b18457d23bb185b734df347f6d697f406c6eb37833dae1e9df624fdd3d91d99ea5f00800700b122a25603445e1dfd11017b226c6561646572223a342c227672665f76616c7565223a22424e45466c6a5833685064496941697447736c77642f464976414a3472324c6b31414861315334376a645441546c677a45796f4e544b6649536237484e456f55524c53744648626a6b2f4a77414d5770693131637073383d222c227672665f70726f6f66223a226635654e4473394734366e775865366b394965786d686838526a4d31555977624a654c614671553173496a4957314f30655936377070426a63316c55773131646346557636346a622f42374b6274485a4670384e31773d3d222c226c6173745f636f6e6669675f626c6f636b5f6e756d223a3437303536352c226e65775f636861696e5f636f6e666967223a6e756c6c7d0000000000000000000000000000000000000000"
		sigs = "0xd468025ac3c5dc05704dce49694d2cb74ff3b4672440fc8b62a90802b119356e6122be5f00e1db3c62be58a9025c34dbb9f843c1741ae6776098aecc8ac3f6cc0191ab3bb2f7056244b761a0a78212e994031d9c7193e49fc30d2ba2ed6f798b8d07a31d40447c0709fab45c0436b1ad3f692e163452cb6e5611e241ece5110eda01c2eba739ba41a0b9d45fde0510e3e677e6d0139f8b00e8ca523b3b77e7f528f27accd670dc420e5ffbe8631e24163acd7ce5a62d4cec1ed39cd76a6d4667f29900612ff730c9207b3fd9fe4a30523e0dd5b82eef75902aaec492aeb03dc81ff9721b96e44038b4b2b7f40b9ab11b23200fe8769a9d1838483822a398e9e2d4657b01"
	)

	header := new(polytypes.Header)
	{
		bz, _ := hexutil.Decode(headerData)
		source := common.NewZeroCopySource(bz)
		_ = header.Deserialization(source)
	}

	toMerkleValue := new(crscm.ToMerkleValue)
	{
		proof, _ := hexutil.Decode(auditPath)
		toMerkleValueBs, err := merkle.MerkleProve(proof, header.CrossStateRoot.ToArray())
		assert.NoError(t, err)

		source := common.NewZeroCopySource(toMerkleValueBs)
		err = toMerkleValue.Deserialization(source)
		assert.NoError(t, err)
	}

	params := toMerkleValue.MakeTxParam
	toContract := pltcm.BytesToAddress(params.ToContractAddress)
	hash := pltcm.BytesToHash(params.TxHash)
	method := params.Method
	toChainID := params.ToChainID
	crsChainID := bytesToUint64(params.CrossChainID)
	input := new(plt.MethodUnlockInput)
	txArgs := new(plt.TxArgs)

	{
		args := params.Args
		ab := plt.GetABI()
		enc, err := utils.PackMethod(ab, method, args, params.FromContractAddress, toMerkleValue.FromChainID)
		assert.NoError(t, err)

		err = utils.UnpackMethod(ab, method, input, enc)
		assert.NoError(t, err)

		src := nvcm.NewZeroCopySource(params.Args)
		assert.NoError(t, txArgs.Deserialization(src))
	}

	t.Logf("method `%s`,\r\n"+
		"hash %s,\r\n"+
		"to contract %s,\r\n"+
		"toChainID %d,\r\n"+
		"cross chainID %d\r\n"+
		"tx args-hash %s\r\n"+
		"tx args-to address %s\r\n"+
		"tx args-amount %d\r\n",
		method, hash.Hex(), toContract.Hex(), toChainID, crsChainID,
		pltcm.BytesToHash(txArgs.ToAssetHash).Hex(),
		pltcm.BytesToAddress(txArgs.ToAddress).Hex(),
		plt.PrintUPLT(txArgs.Amount))

	// toMerkleValue.MakeTxParam.Args

	{
		// todo:
		//raw, err := testPolyMgr.eccd.GetCurEpochConPubKeyBytes(nil)
		//assert.NoError(t, err)
		//bookeepers := deserializeKeepers(raw)

		dec, err := hexutil.Decode(headerData)
		assert.NoError(t, err)
		hdr, err := polytypes.HeaderFromRawBytes(dec)
		assert.NoError(t, err)
		hash := hdr.Hash()

		sigData, err := hexutil.Decode(sigs)
		assert.NoError(t, err)
		sig, err := dissembleHeaderSigs(sigData)
		assert.NoError(t, err)

		err = signature.VerifyMultiSignature(
			hash[:],
			nil, //bookeepers,
			len(hdr.Bookkeepers),
			sig,
		)
		assert.NoError(t, err)
	}
}

func TestPolyCommitProof(t *testing.T) {
	var (
		auditPath  = "0xef20dc8dc52c8a8f388426822d6f2908667dc83137692e53d1aece82cc149dd12c0e6b0000000000000020000000000000000000000000000000000000000000000000000000000000000020ff375bc7b8c8da9cf5e43ed89bc2822b2921bfa4fc66edb1a2a1893b4ddcc6531400000000000000000000000000000000000001036b0000000000000014000000000000000000000000000000000000010306756e6c6f636b4a140000000000000000000000000000000000000103145593b2b8dc63d0ed68aa8f885707b2dc5787e391000064a7b3b6e00d000000000000000000000000000000000000000000000000"
		headerData = "0x00000000db056dd1000000000a477ff2da6d87da01cf6dc6406baf3df5c055dd0b87b85d89667868dad09cb400000000000000000000000000000000000000000000000000000000000000000c3b0ca0e1731299f6a4ede160bd7f3cc562b849c5b6b1f7b664c22c8c9e5c2c8a87b41db1bac796e20c378b65fe138cd0a640facf6ed8969773854427c5fcd490f5ea5f2e8307007fe979dd2166b331fd11017b226c6561646572223a312c227672665f76616c7565223a2242432f5938374a6f386a41646158394e4451774f6e4e4d5662704a56324269335a32795177477131706d7961426e67665a69666f54313072396d74614a5672326e6e6356596b74686272376b32364637502f513155354d3d222c227672665f70726f6f66223a22365a582b6f71496e2f7a5955547377396348654138765855435769346f50366b784d6c2f4661724c35463631702b536661377550636c717043566d696a464d593933322b315a4e6369384e6e7930766a316c784a6c773d3d222c226c6173745f636f6e6669675f626c6f636b5f6e756d223a3437303536352c226e65775f636861696e5f636f6e666967223a6e756c6c7d0000000000000000000000000000000000000000"
		rawProof   = "0x"
		rawAnchor  = "0x"
		sigs       = "0x475b98076ab1409c15f3b586e8d22ce8f08d4d334ed8b66025f5dac9cee16bb92cefe64744325d0b4ce793cc130b04ee7d1593f1d44a2a49a7779e4abc56ef7d01796fea842db88c6763a96a0253933da266bb9f39b832bf2047c8b804f2f0f0a9620e2d1089484b408b546e71e5e947038a4cd1afa0c2b6741ffea50345f6008f0135f74c66aba4960454e99ef6e7060c7b5674ebde554389664e2a2b507d28e8256caa0963497b574921b6be410f48558fde1c1985f5a92d2abfb34e01a9f3eca30148b640c5fff2c350a717225ddecfe21617795ac3d4fa5d16eff9c1c61fefc64859d63fbf2a8bd373454428985808d6770d711d39287cd13623671b843777d78100"
	)
	auditPathBytes, _ := hexutil.Decode(auditPath)
	headerBytes, _ := hexutil.Decode(headerData)
	rawProofBytes, _ := hexutil.Decode(rawProof)
	rawAnchorBytes, _ := hexutil.Decode(rawAnchor)
	sigBytes, _ := hexutil.Decode(sigs)

	sender := testPolyMgr.selectSender()

	txData, err := sender.contractAbi.Pack(
		"verifyHeaderAndExecuteTx",
		auditPathBytes,
		headerBytes,
		rawProofBytes,
		rawAnchorBytes,
		sigBytes,
	)
	assert.NoError(t, err)

	//txinfo := &PaletteTxInfo{
	//	txData:       txData,
	//	contractAddr: sender.eccmContract(),
	//	gasPrice:     gasPrice,
	//	gasLimit:     gasLimit,
	//	polyTxHash:   polyTxHash,
	//}

	err = sender.sendTxToPalette(
		sender.eccmContract(),
		"test",
		txData,
	)
	assert.NoError(t, err)
}
