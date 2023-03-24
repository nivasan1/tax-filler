package taxfiller

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

const (
	taxDataQuery = `
	SELECT
		txs,
		auction_timestamp,
		address_submitted,
		(SELECT moniker from validators where validators.cons_address = winning_bundles.cons_address),
		(SELECT oper_address from validators where validators.cons_address = winning_bundles.cons_address),
		val_profit,
		net_profit,
		winning_bundles.height
	from winning_bundles INNER JOIN val_profits ON winning_bundles.height = val_profits.height
	WHERE val_profits.timestamp < '2023-01-01'  AND
	winning_bundles.height >= COALESCE($1, 0) ORDER BY winning_bundles.height ASC ;
	`
	dbName = "peersdb"
	dbUser = "postgres"
	dbPort = "5432"
)

type RowData struct {
	txs              string
	auctionTimestamp time.Time
	addressSubmitted string
	moniker          string
	validatorAddr    string
	valProfit        int64
	valFees          int64
	height           int64
}

type DB interface {
	Query(int64) ([]RowData, error)
}

type PeersDB struct {
	conn *pgx.Conn
}

func NewPeersDB(password, host string) *PeersDB {
	fmt.Println(fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s",
		dbUser,
		password,
		host,
		dbPort,
		dbName,
	))
	config, err := pgx.ParseConfig(fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s",
		dbUser,
		password,
		host,
		dbPort,
		dbName,
	))
	if err != nil {
		fmt.Println("error in starting peer db", err)
		return nil
	}
	conn, err := pgx.ConnectConfig(context.Background(), config)
	if err != nil {
		fmt.Println("error in connecting to db", err)
		return nil
	}
	return &PeersDB{
		conn: conn,
	}
}

func (p PeersDB) Query(height int64) ([]RowData, error) {
	rows, err := p.conn.Query(context.Background(), taxDataQuery, height)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rowDatas := make([]RowData, 0)
	for rows.Next() {
		rowData := RowData{}
		if err := rows.Scan(&rowData.txs, &rowData.auctionTimestamp, &rowData.addressSubmitted, &rowData.moniker, &rowData.validatorAddr, &rowData.valProfit, &rowData.valFees, &rowData.height); err != nil {
			return nil, err
		}
		rowDatas = append(rowDatas, rowData)
	}
	return rowDatas, nil
}
