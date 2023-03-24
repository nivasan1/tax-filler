package main

import (
	"flag"
	"github.com/skip-mev/tax-filler"
	"log"
	"os"
)

var (
	home = flag.String("home", ".", "address in which to look for config")
)

func main() {
	// get flags
	flag.Parse()
	// get password if it exists
	pass := os.Getenv("PGPASSWORD")
	config, err := taxfiller.FillConfig(*home, pass)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(*config)
	signals := make(chan struct{}, len(*config))
	// create / run tax-filler
	for chainId := range *config {
		go func(chainId string) {
			tf := taxfiller.NewTaxFiller(*config, chainId)
			if err := tf.FillTaxData(); err != nil {
				log.Fatal(err)
			}
			signals <- struct{}{}
		}(chainId)
	}
	for {
		if len(signals) == cap(signals) {
			break
		}
	}
}
