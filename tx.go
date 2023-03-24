package taxfiller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	encoding "github.com/evmos/ethermint/encoding/codec"
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
	encoding.RegisterInterfaces(reg)
	txCodec = codec.NewProtoCodec(reg)
}

// needs to be able to check if the tx is designated to one of our auction-house addresses
type Tx interface {
	// Given some bytes, check whether the given tx is designated to our auction house address
	// Returns:
	//  - amount sent
	//  - fee amount
	//  - if the tx was a test tx
	CheckAuctionFeeTx(string) (int64, int64)
	// Given a txhash (corresponding to a validator payment), check that the sender is the auction house addresss
	// and return the receiver
	CheckValPaymentTx(string, string) string
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

func (t TxChecker) CheckValPaymentTx(txhash string, skipAddress string) string {
	tx := txBytes(t.nodeAddr, txhash)
	if tx == nil {
		return ""
	}
	feeTx := getFeeTx(tx)
	if feeTx == nil {
		fmt.Println("error unmarshalling tx", txhash)
		return ""
	}
	// check that the fromAddress is the skipAddress
	for _, msg := range feeTx.GetMsgs() {
		if bankMsg, ok := msg.(*banktypes.MsgSend); ok {
			if bankMsg.FromAddress == skipAddress {
				fmt.Println("found val-payment TX!!")
				return bankMsg.ToAddress
			}
		}
	}
	return ""
}

func (t TxChecker) CheckAuctionFeeTx(txhash string) (int64, int64) {
	tx := txBytes(t.nodeAddr, txhash)
	if tx == nil {
		return 0, 0
	}
	feeTx := getFeeTx(tx)
	if feeTx == nil {
		fmt.Println("error unmarshalling tx", txhash)
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

func getFeeTx(txBytes []byte) sdk.FeeTx {
	txRaw, err := authtx.DefaultTxDecoder(txCodec)(txBytes)
	if err != nil {
		fmt.Println("error unmarshalling tx: ", err)
		return nil
	}
	// check that the tx is a fee tx (all sdk txs are)
	feeTx, ok := txRaw.(sdk.FeeTx)
	if !ok {
		return nil
	}
	return feeTx
}

func txBytes(nodeAddr, txHash string) []byte {
	req := fmt.Sprintf("%s/tx?hash=0x%s", nodeAddr, txHash)
	fmt.Println(req)
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
