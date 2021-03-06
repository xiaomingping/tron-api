package rpc

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/fbsobreira/gotron-sdk/pkg/common"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/shopspring/decimal"
	"github.com/xiaomingping/tron-api/pkg/base58"
	"github.com/xiaomingping/tron-api/pkg/crypto"
	"github.com/xiaomingping/tron-api/pkg/hexutil"
	"github.com/xiaomingping/tron-api/pkg/model"
	"github.com/xiaomingping/tron-api/pkg/sign"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"log"
	"math/big"
	"math/rand"
	"sync"
	"time"
)

var (
	curIndex        = 0
	mutex           sync.Mutex
	conn     *grpc.ClientConn
	feeLimit int64 = 40000000 // 转账合约燃烧 trx数量 单位 sun 默认0.5trx 转账一笔大概消耗能量 0.26trx
	Trx            = "trx"
	Urls           = []string{
		"grpc.trongrid.io",
		"grpc.trongrid.io",
		"grpc.trongrid.io",
		"grpc.trongrid.io",
	}
	trxDecimal      int32 = 6 // trx 单位
	mapContractType       = map[string]bool{
		"trx":   true,
		"trc10": true,
		"trc20": true,
	}
	mapContract = make(map[string]*model.ContractModel)
)

func SetContractMap(ContractMap map[string]*model.ContractModel) {
	mapContract = ContractMap
}

type Rpc struct{
	apiKeys  []string // api key
}

// 判断当前属于什么合约
func ChargeContract(contract string) (string, int32) {
	if contract == "trx" || contract == "" {
		return Trx, trxDecimal
	}
	if v := mapContract[contract]; v != nil {
		if ok, _ := mapContractType[v.Type]; ok {
			return v.Type, v.Decimal
		}
	}
	return "NONE", 18
}
func NewRpc(apiKeys []string) *Rpc {
	return &Rpc{apiKeys: apiKeys}
}

func (r *Rpc) getIp() string {
	return Urls[rand.Intn(len(Urls))] + ":50051"
}
// 获取api key
func (c *Rpc) getApiKey() string {
	mutex.Lock()
	defer mutex.Unlock()
	lens := len(c.apiKeys)
	if curIndex >= lens {
		curIndex = 0
	}
	inst := c.apiKeys[curIndex]
	curIndex = (curIndex + 1) % lens
	return inst
}

func (r *Rpc) creationConn() (*grpc.ClientConn, error) {
	Conn, err := grpc.Dial(
		r.getIp(),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             100 * time.Millisecond,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return Conn, nil
}

// 获取新节点
func (r *Rpc) getNode() (*grpc.ClientConn, error) {
	if conn != nil {
		if conn.GetState() == connectivity.Shutdown {
			return r.creationConn()
		}
		return conn, nil
	}
	return r.creationConn()
}

// 获取超时上下文
func (r *Rpc) timeoutContext() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	go func() {
		time.Sleep(time.Second * 30)
		cancel()
	}()
	ctx = metadata.AppendToOutgoingContext(ctx, "TRON-PRO-API-KEY", r.getApiKey())
	return ctx
}

// 获取客户端
func (r *Rpc) GetClient() api.WalletClient {
	Conn, err := r.getNode()
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}
	return api.NewWalletClient(Conn)
}

// trx转账
func (r *Rpc) transfer(ownerKey *ecdsa.PrivateKey, toAddress string, amount int64) (string, error) {
	transferContract := new(core.TransferContract)
	transferContract.OwnerAddress = crypto.PubkeyToAddress(ownerKey.
		PublicKey).Bytes()
	transferContract.ToAddress, _ = base58.DecodeCheck(toAddress)
	transferContract.Amount = amount

	transferTransactionEx, err := r.GetClient().CreateTransaction2(r.timeoutContext(), transferContract)

	var txid string
	if err != nil {
		return txid, err
	}
	transferTransaction := transferTransactionEx.Transaction
	if transferTransaction == nil || len(transferTransaction.
		GetRawData().GetContract()) == 0 {
		return txid, fmt.Errorf("transfer error: invalid transaction")
	}
	hash, err := sign.SignTransaction(transferTransaction, ownerKey)
	if err != nil {
		return txid, err
	}
	txid = hexutil.Encode(hash)

	result, err := r.GetClient().BroadcastTransaction(r.timeoutContext(),
		transferTransaction)
	if err != nil {
		return "", err
	}
	if !result.Result {
		return "", fmt.Errorf("api get false the msg: %v", result.String())
	}
	return txid, err
}

// 合约转账 TRC20
func (r *Rpc) transferContract(ownerKey *ecdsa.PrivateKey, Contract string, data []byte, feeLimit int64) (string, error) {
	transferContract := new(core.TriggerSmartContract)
	transferContract.OwnerAddress = crypto.PubkeyToAddress(ownerKey.
		PublicKey).Bytes()
	transferContract.ContractAddress, _ = base58.DecodeCheck(Contract)
	transferContract.Data = data
	transferTransactionEx, err := r.GetClient().TriggerConstantContract(r.timeoutContext(), transferContract)
	var txid string
	if err != nil {
		return txid, err
	}
	transferTransaction := transferTransactionEx.Transaction
	if transferTransaction == nil || len(transferTransaction.
		GetRawData().GetContract()) == 0 {
		return txid, fmt.Errorf("transfer error: invalid transaction")
	}
	if feeLimit > 0 {
		transferTransaction.RawData.FeeLimit = feeLimit
	}

	hash, err := sign.SignTransaction(transferTransaction, ownerKey)
	if err != nil {
		return txid, err
	}
	txid = hexutil.Encode(hash)

	result, err := r.GetClient().BroadcastTransaction(r.timeoutContext(),
		transferTransaction)
	if err != nil {
		return "", err
	}
	if !result.Result {
		return "", fmt.Errorf("api get false the msg: %v", result.String())
	}
	return txid, err
}

// 处理合约转账参数
func (r *Rpc) processTransferParameter(to string, amount int64) (data []byte) {
	methodID, _ := hexutil.Decode("a9059cbb")
	addr, _ := base58.DecodeCheck(to)
	paddedAddress := common.LeftPadBytes(addr[1:], 32)
	amountBig := new(big.Int).SetInt64(amount)
	paddedAmount := common.LeftPadBytes(amountBig.Bytes(), 32)
	data = append(data, methodID...)
	data = append(data, paddedAddress...)
	data = append(data, paddedAmount...)
	return
}

// 转账
func (r *Rpc) Sen(key *ecdsa.PrivateKey, contract, to string, amount decimal.Decimal) (string, error) {
	Type, Decimal := ChargeContract(contract)
	switch Type {
	case Trx:
		var amountdecimal = decimal.New(1, Decimal)
		amountac, _ := amount.Mul(amountdecimal).Float64()
		return r.transfer(key, to, int64(amountac))
	case "trc20":
		var amountdecimal = decimal.New(1, Decimal)
		amountac, _ := amount.Mul(amountdecimal).Float64()
		data := r.processTransferParameter(to, int64(amountac))
		return r.transferContract(key, contract, data, feeLimit)
	case "trc10":
		return "", nil
	default:
		return "", nil
	}
}
