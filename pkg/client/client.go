package client

import (
	"errors"
	"fmt"
	"github.com/gogf/gf/frame/g"
	"github.com/gogf/gf/net/ghttp"
	"github.com/gogf/gf/util/gconv"
	"github.com/shopspring/decimal"
	"sync"
)

const (
	ApiUrl       = "https://api.trongrid.io"        // 主网
	ApiUrlShasta = "https://api.shasta.trongrid.io" // Shasta测试网
)

var (
	curIndex        = 0
	mutex           sync.Mutex
	ApiKeys         []string
	trxdecimal      int32 = 6 // trx 单位
	mapContract           = make(map[string]*ContractModel)
	mapContractType       = map[string]bool{
		"trx":   true,
		"trc10": true,
		"trc20": true,
	}
)

// 初始化合约
func InitContract(contracts []ContractModel) error {
	for i, v := range contracts {
		if ok, _ := mapContractType[v.Type]; ok {
			mapContract[v.Contract] = &contracts[i]
		} else {
			return fmt.Errorf("the contract type %s is not exist pleasecheck", v.Type)
		}
	}
	return nil
}

// 判断当前属于什么合约
func chargeContract(contract string) (string, int32) {
	if contract == "trx" || contract == "" {
		return Trx, trxdecimal
	}
	if v := mapContract[contract]; v != nil {
		if ok, _ := mapContractType[v.Type]; ok {
			return v.Type, v.Decimal
		}
	}
	return "NONE", 18
}

func chargeContractObj(contract string) *ContractModel {
	if v := mapContract[contract]; v != nil {
		return v
	}
	return nil
}

// 5527c743-dc35-4a00-8b97-7e75ac9c164b
// 4c492539-5e03-452b-9633-6e5b8998cc36
type Client struct {
	Url string
}

func NewClient() *Client {
	return &Client{Url: ApiUrl}
}

// 获取请求客户端
func (c *Client) getClient() *ghttp.Client {
	Client := ghttp.NewClient()
	Client.SetHeader("Content-Type", "application/json")
	Client.SetHeader("TRON-PRO-API-KEY", c.getApiKey())
	return Client
}

// 获取api key
func (c *Client) getApiKey() string {
	mutex.Lock()
	defer mutex.Unlock()
	lens := len(ApiKeys)
	if curIndex >= lens {
		curIndex = 0
	}
	inst := ApiKeys[curIndex]
	curIndex = (curIndex + 1) % lens
	return inst
}

// 获取用户信息
func (c *Client) GetAccount(address string) (*GetAccountModel, error) {
	url := fmt.Sprintf("%s/v1/accounts/%s", c.Url, address)
	body := c.getClient().GetVar(url)
	if body.IsEmpty() {
		return nil, errors.New("网络错误")
	}
	var Account RespAccount
	err := body.Struct(&Account)
	if err != nil {
		g.Log().Error(err)
		return nil, err
	}
	if Account.Success != true {
		return nil, errors.New("连接失败")
	}
	if Account.Data == nil {
		return nil, errors.New("账号未激活")
	}
	return &Account.Data[0], nil
}

// 进度转换
func BalanceAccuracy(Balance string, exp int32) string {
	b, _ := decimal.NewFromString(Balance)
	return b.Mul(decimal.New(1, exp)).String()
}

// 获取余额
func GetTRXBalance(req *GetAccountModel) map[string]string {
	BalanceModel := make(map[string]string)
	Balance := gconv.Int64(req.Balance)
	BalanceModel[Trx] = BalanceAccuracy(gconv.String(Balance), -trxdecimal)
	for _, v := range req.Trc20 {
		for key, val := range v {
			if v :=chargeContractObj(key);v != nil {
				BalanceModel[v.Name] = BalanceAccuracy(gconv.String(val), -v.Decimal)
			}
		}
	}
	return BalanceModel
}

// 获取账户历史TRC20交易记录
func (c *Client) GetTransactionsTrc20(address, contract string, ) ([]TransactionsTrc20Model, error) {
	url := fmt.Sprintf("%s/v1/accounts/%s/transactions/trc20?only_confirmed=true&only_to=true&contract_address=%s", c.Url, address, contract)
	body := c.getClient().GetVar(url)
	if body.IsEmpty() {
		return nil, errors.New("网络错误")
	}
	var TransactionsTrc20 RespTransactionsTrc20
	err := body.Struct(&TransactionsTrc20)
	if err != nil {
		g.Log().Error(err)
		return nil, err
	}
	if TransactionsTrc20.Success != true {
		return nil, errors.New("连接失败")
	}
	return TransactionsTrc20.Data, nil
}

// 获取区块详情
func (c *Client) GetBlockById(exchangeId string) (*GettransactioninfobyidModel, error) {
	url := fmt.Sprintf("%s/wallet/gettransactioninfobyid", c.Url)
	body := c.getClient().PostVar(url, g.Map{"value": exchangeId})
	if body.IsEmpty() {
		return nil, errors.New("网络错误")
	}
	var GettransactioninfobyidModel GettransactioninfobyidModel
	err := body.Struct(&GettransactioninfobyidModel)
	if err != nil {
		g.Log().Error(err)
		return nil, err
	}
	return &GettransactioninfobyidModel, nil
}
