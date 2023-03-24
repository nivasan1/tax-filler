package taxfiller

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// spawns workers number of threads to begin collecting data and posting to redis, once all threads have finished their work
// the co-ordinator process reads all data from redis and writes to csv
type TaxFiller struct {
	skipAddress string
	chainID     string
	token       string
	// Tx interface for checking / extracting data from txs in message
	checker Tx
	// DB interface for querying data from the DB
	db DB
	// Store interface for storing data between cycles
	store Store
	// set of test accounts for this chain
	testAccts map[string]struct{}
	// number of sub-threads to spin up in aggregating data
	workers int
}

type ThreadSafeRowData struct {
	sync.RWMutex
	data []RowData
}

func (t *ThreadSafeRowData) Len() int {
	t.RLock()
	defer t.RUnlock()
	return len(t.data)
}

func (t *ThreadSafeRowData) Get(i int) RowData {
	t.RLock()
	defer t.RUnlock()
	return t.data[i]
}

func NewTaxFiller(config Config, chainID string) TaxFiller {
	configForChain, ok := config[chainID]
	if !ok {
		return TaxFiller{}
	}
	testAccts := make(map[string]struct{})
	for _, acct := range configForChain.TestAccts {
		testAccts[acct] = struct{}{}
	}
	return TaxFiller{
		skipAddress: configForChain.SkipAddress,
		chainID:     chainID,
		token:       configForChain.Token,
		checker:     NewTxChecker(configForChain.NodeRPC, configForChain.SkipAddress),
		db:          NewPeersDB(configForChain.DBPassword, configForChain.DBHost),
		store:       NewRedisClient(configForChain.RedisAddr),
		testAccts:   testAccts,
		workers:     configForChain.Threads,
	}
}

func (t *TaxFiller) FillTaxData() error {
	// read latest height from redis
	height, err := t.store.GetLatestHeight(t.chainID)
	if err != nil {
		return err
	}
	log.Println("starting from height", height)
	// get row-data from the DB, according to the latest height queried
	rowData, err := t.db.Query(height)
	if err != nil {
		return err
	}
	fmt.Println("received", len(rowData), "rows from db")
	// create signal channels for all workers, and identify when finished
	signals := make(chan struct{}, t.workers)
	trd := &ThreadSafeRowData{
		data: rowData,
	}
	for i := 0; i < t.workers; i++ {
		go t.workerFunc(i, t.workers, trd, signals)
	}
	// wait until channel capacity is reached
	for {
		if len(signals) == cap(signals) {
			break
		}
	}
	tds, err := t.store.GetAllTaxData(t.chainID)
	if err != nil {
		return err
	}
	return writeCSV(tds, fmt.Sprintf("%s.csv", t.chainID))
}

func (t *TaxFiller) workerFunc(index, indexIncrement int, rows *ThreadSafeRowData, signal chan struct{}) {
	stopAt := rows.Len()
	// when finished with work, signal main thread
	defer func() {
		signal <- struct{}{}
	}()
	for {
		// update the index by increment
		index += indexIncrement
		if index >= stopAt {
			break
		}
		// get row data at index
		row := rows.Get(index)
		// check if this was a test bundle
		_, test := t.testAccts[row.addressSubmitted]
		txs := strings.Split(row.txs, ",")
		if len(txs) == 0 {
			fmt.Printf("bundle at height %d with no txs\n", row.height)
			continue
		}
		// check the val-payment tx
		if paymentAddr := t.checker.CheckValPaymentTx(txs[len(txs)-1], t.skipAddress); paymentAddr != "" {
			// set the tax data in redis for the current bundle (the first will be the valPayment)
			if err := t.store.Set(calculateTaxesForValidatorPayment(row, txs[len(txs)-1], t.chainID, t.token, t.skipAddress, paymentAddr, test)); err != nil {
				fmt.Println("error setting tax data in redis:", err)
				return
			}
		}
		// iterate over txs, and check if the tx was an auction payment
		amtSent, amtFees := int64(0), int64(0)
		for _, tx := range txs {
			amtSent, amtFees = t.checker.CheckAuctionFeeTx(tx)
			if amtSent != 0 || amtFees != 0 {
				break
			}
		}
		// set the last auction data in redis
		if err := t.store.Set(calculateTaxesForAuctionFee(row, txs[len(txs)-1], t.chainID, t.token, t.skipAddress, test, amtSent, amtFees)); err != nil {
			fmt.Println("error setting tax data in redis:", err)
			return
		}
	}
	return
}

func calculateTaxesForValidatorPayment(row RowData, tx, chainID, token, skipAddr, paymentAddr string, test bool) TaxData {
	return rowDataToTaxData(row, tx, chainID, token, skipAddr, paymentAddr, test, row.valProfit, row.valFees)
}

func calculateTaxesForAuctionFee(row RowData, tx, chainID, token, skipAddr string, test bool, amtSent, fees int64) TaxData {
	return rowDataToTaxData(row, tx, chainID, token, row.addressSubmitted, skipAddr, test, amtSent, fees)
}

func rowDataToTaxData(row RowData, tx, chainID, token, sender, receiver string, test bool, amtSent, fees int64) TaxData {
	return TaxData{
		ChainID:         chainID,
		TxHash:          tx,
		Date:            row.auctionTimestamp,
		SenderAddress:   sender,
		ReceiverAddress: receiver,
		Moniker:         row.moniker,
		Token:           token,
		Amount:          amtSent,
		FeeAmount:       fees,
		Height:          row.height,
		Test:            test,
	}
}

func writeCSV(tds []TaxData, outname string) error {
	// create file
	file, err := os.Create(outname)
	if err != nil {
		return err
	}
	csvFile := csv.NewWriter(file)
	if err := csvFile.Write([]string{"tx_id", "date", "sender_address", "receiver_address", "moniker", "token", "amount", "fee_token", "fee_amount", "test"}); err != nil {
		return err
	}

	for _, td := range tds {
		// create rows
		if err := csvFile.Write(td.ToRecord()); err != nil {
			fmt.Println(err)
			return err
		}
	}
	// flush buffer to ensure all records are written
	csvFile.Flush()
	if err := csvFile.Error(); err != nil {
		fmt.Println("error flushing to disk: ", err)
	}
	return nil
}
