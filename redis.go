package taxfiller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"sort"

	redis "github.com/redis/go-redis/v9"
)

type TaxData struct {
	ChainID         string
	TxHash          string
	Date            time.Time
	SenderAddress   string
	ReceiverAddress string
	Moniker         string
	Token           string
	Amount          int64
	FeeAmount       int64
	Height          int64
	Test            bool
}

func (t TaxData) String() string {
	return strings.Join([]string{
		t.ChainID,
		t.TxHash,
		t.Date.String(),
		t.SenderAddress,
		t.ReceiverAddress,
		t.Moniker,
		t.Token,
		strconv.Itoa(int(t.Amount)),
		strconv.Itoa(int(t.FeeAmount)),
		strconv.Itoa(int(t.Height)),
		strconv.FormatBool(t.Test)},
		",",
	)
}

func (t TaxData) ToRecord() []string {
	return []string{
		t.TxHash,
		t.Date.String(),
		t.SenderAddress,
		t.ReceiverAddress,
		t.Moniker, 
		t.Token,
		strconv.Itoa(int(t.Amount)),
		t.Token,
		strconv.Itoa(int(t.FeeAmount)),
		strconv.FormatBool(t.Test),
	}
}

func stringToTaxData(s string) *TaxData {
	datas := strings.Split(s, ",")
	date, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", datas[2])
	if err != nil {
		fmt.Println("error parsing tax-data:", err)
		return nil
	}
	amount, err := strconv.Atoi(datas[7])
	if err != nil {
		fmt.Println("error parsing tax-data:", err)
		return nil
	}
	feeAmount, err := strconv.Atoi(datas[8])
	if err != nil {
		fmt.Println("error parsing tax-data:", err)
		return nil
	}
	height, err := strconv.Atoi(datas[9])
	if err != nil {
		fmt.Println("error parsing tax-data:", err)
		return nil
	}
	test, err := strconv.ParseBool(datas[10])
	if err != nil {
		fmt.Println("error parsing tax-data:", err)
		return nil
	}
	return &TaxData{
		ChainID:         datas[0],
		TxHash:          datas[1],
		Date:            date,
		SenderAddress:   datas[3],
		ReceiverAddress: datas[4],
		Moniker:         datas[5],
		Token:           datas[6],
		Amount:          int64(amount),
		FeeAmount:       int64(feeAmount),
		Height:          int64(height),
		Test:            test,
	}
}

type Store interface {
	Set(TaxData) error
	GetLatestHeight(chainID string) (int64, error)
	GetAllTaxData(chainID string) ([]TaxData, error)
}

func NewRedisClient(addr string) *RedisClient {
	return &RedisClient{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: "",
			DB:       0,
		}),
	}
}

type RedisClient struct {
	client *redis.Client
}

// Set TaxData in redis, key will be fmt.Sprintf(t.Height)||senderAddress (as each height will have two unique interactions (one send, one receive))
func (r RedisClient) Set(t TaxData) error {
	// format tax-data as comma-delimited string, key value will be a random string,
	return r.client.Set(context.Background(), fmt.Sprintf("%s|%d|%s", t.ChainID, t.Height, t.SenderAddress), t.String(), 0).Err()
}

func (r RedisClient) GetAllTaxData(chainID string) ([]TaxData, error) {
	// get iterator over all values in redis store, with key prefixed by chain_id
	iter := r.client.Scan(context.Background(), 0, fmt.Sprintf("%s*", chainID), 0).Iterator()
	if iter.Err() != nil {
		return nil, iter.Err()
	}
	taxDatas := make([]TaxData, 0)
	for iter.Next(context.Background()) {
		taxData, err := r.client.Get(context.Background(), iter.Val()).Result()
		if err != nil {
			return nil, err
		}
		t := stringToTaxData(taxData)
		if t != nil {
			taxDatas = append(taxDatas, *t)
		}
	}
	sort.Slice(taxDatas, func(i, j int) bool {
		return taxDatas[i].Height > taxDatas[j].Height
	})

	return taxDatas, nil
}

func (r RedisClient) GetLatestHeight(chainID string) (int64, error) {
	taxDatas, err := r.GetAllTaxData(chainID)
	if err != nil {
		return 0, err
	}
	if len(taxDatas) == 0 {
		fmt.Println("no tax datas found!!")
		return 0, nil
	}
	return taxDatas[0].Height, nil
}
