package taxfiller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"io/ioutil"
	"net/http"
)

var (
	txCodec *codec.ProtoCodec
)

func init() {
	reg := codectypes.NewInterfaceRegistry()
	banktypes.RegisterInterfaces(reg)
	cryptocodec.RegisterInterfaces(reg)
	wasmtypes.RegisterInterfaces(reg)
	txCodec = codec.NewProtoCodec(reg)
}

// needs to be able to check if the tx is designated to one of our auction-house addresses
type Tx interface {
	// Given some bytes, check whether the given tx is designated to our auction house address
	// Returns:
	//  - amount sent
	//  - fee amount
	//  - if the tx was a test tx
	CheckTx(string) (int64, int64)
}

type TxChecker struct {
	auctionHouseAddress string
	nodeAddr            string
}

func NewTxChecker(nodeAddr, auctionHouse string) *TxChecker {
	return &TxChecker{
		auctionHouseAddress: auctionHouse,
		nodeAddr:            nodeAddr,
	}
}

func (t TxChecker) CheckTx(txhash string) (int64, int64) {
	tx := txBytes(t.nodeAddr, txhash)
	if tx == nil {
		return 0, 0
	}
	txRaw, err := authtx.DefaultTxDecoder(txCodec)(tx)
	if err != nil {
		fmt.Println("error unmarshalling tx: ", txhash, err)
		return 0, 0
	}
	// check that the tx is a fee tx (all sdk txs are)
	feeTx, ok := txRaw.(sdk.FeeTx)
	if !ok {
		return 0, 0
	}
	// iterate over messages in tx, and check if message type is a send
	for _, msg := range feeTx.GetMsgs() {
		if bankMsg, ok := msg.(*banktypes.MsgSend); ok {
			// this is a send, and the to address is the auction house
			if len(bankMsg.Amount) > 0 && bankMsg.ToAddress == t.auctionHouseAddress {
				return bankMsg.Amount[0].Amount.Int64(), feeTx.GetFee()[0].Amount.Int64()
			}
		}
		continue
	}
	return 0, 0
}

func txBytes(nodeAddr, txHash string) []byte {
	req := fmt.Sprintf("%s/tx?hash=0x%s", nodeAddr, txHash)
	resp, err := http.Get(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error reading response body:", err)
		return nil
	}
	var m map[string]interface{}
	if json.Unmarshal(bytes, &m) != nil {
		fmt.Println("error unmarshaling response body:", err)
		return nil
	}
	jsonResp, ok := m["result"].(map[string]interface{})
	if !ok {
		fmt.Println("result incorrectly formatted", m)
		return nil
	}
	base64Tx, ok := jsonResp["tx"].(string)
	if !ok {
		fmt.Println("error retrieving response from", m["result"])
		return nil
	}
	bytes, err = base64.StdEncoding.WithPadding(base64.StdPadding).DecodeString(base64Tx)
	if err != nil {
		fmt.Println("error unmarshalling base64 tx bytes:", err)
		return nil
	}
	return bytes
}
