package BlockChain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks_%s"
	genesisData = "First Transaction from Genesis"

)



type BlockChain struct{
	LastHash []byte
	Database *badger.DB
}



func (chain *BlockChain) MineBlock(transactions []*Transaction) *Block{
	var lastHash []byte
	var lastHeight int


	for _, tx := range transactions{
		if chain.VerifyTransaction(tx) != true{
			log.Panic("invalid transaction")
		}
	}
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)
		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)
		lastBlock := Deserialize(lastBlockData)
		lastHeight = lastBlock.Height
		return err
	})
	Handle(err)
	newBlock :=CreateBlock(transactions, lastHash, lastHeight + 1)
	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		Handle(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash
		return err
	})
	Handle(err)
	return newBlock
}

func (chain *BlockChain) GetBestHeight() int {
	var lastBlock Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock = *Deserialize(lastBlockData)

		return nil
	})
	Handle(err)

	return lastBlock.Height
}



func (chain *BlockChain) AddBlock(block *Block){
	err := chain.Database.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(block.Hash); err != nil{
			return nil
		}
		blockData := block.Serialize()
		err := txn.Set(block.Hash, blockData)
		Handle(err)

		item, err := txn.Get([]byte("ln"))
		Handle(err)
		last_hash, _ := item.ValueCopy(nil)
		item, err = txn.Get(last_hash)
		Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)
		lastBlock := Deserialize(lastBlockData)

		if block.Height> lastBlock.Height{
			err = txn.Set([]byte("ln"), block.Hash)
			Handle(err)
			chain.LastHash = block.Hash
		}
		return nil
	})
	Handle(err)
}

func (chain *BlockChain) GetBlock(blockHast []byte)(Block, error){
	var block Block
	err := chain.Database.View(func(txn *badger.Txn) error {
		if item, err := txn.Get(blockHast); err != nil{
			return errors.New("Block is not found")
		}else{
			blockData, _ := item.ValueCopy(nil)
			block = *Deserialize(blockData)
		}
		return nil
	})
	if err != nil{
		return block, err
	}
	return block, nil
}

func (chain *BlockChain) GetBlockHashes() [][]byte {
	var blocks [][]byte

	iter := chain.Iterator()

	for {
		block := iter.Next()

		blocks = append(blocks, block.Hash)

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}
func InitBlockChain(address string, nodeId string) *BlockChain {
	var lastHash []byte
	path := fmt.Sprintf(dbPath, nodeId)
	if DBexists(path) {
		fmt.Println("BlockChain already exists")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := openDB(path, opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte("lh")); err == badger.ErrKeyNotFound {
			cbtx := CoinbaseTx(address, genesisData)
			genesis := Genesis(cbtx)
			fmt.Println("Genesis is Created")
			err = txn.Set(genesis.Hash, genesis.Serialize())
			Handle(err)
			err = txn.Set([]byte("lh"), genesis.Hash)

			lastHash = genesis.Hash
			return err
		} else {
			item, err := txn.Get([]byte("lh"))
			Handle(err)
			lastHash, err = item.ValueCopy(nil)
			return err
		}
	})

	Handle(err)

	blockchain := BlockChain{lastHash, db}
	return &blockchain
}




func DBexists(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}
	return true
}

func ContinueBlockChain(nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)
	if DBexists(path) == false {
		fmt.Println("No existing blockchain, create one")
		runtime.Goexit()
	}

	var lastHash []byte

	opts:= badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath	
	db, err := openDB(path, opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)

		return err
	})

	Handle(err)
	chain := BlockChain{lastHash, db}
	return &chain
}




func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block:= iter.Next()
		for _, tx := range block.Transactions{
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs{
				if spentTXOs[txID] != nil{
					for _, spentOut := range spentTXOs[txID]{
							if spentOut == outIdx{
								continue Outputs
							}
						}
					}

					outs := UTXO[txID]
					outs.Outputs = append(outs.Outputs, out)
					UTXO[txID] = outs
				}
				if tx.IsCoinbase() == false{
					for _, in := range tx.Inputs{
						inTxID := hex.EncodeToString(in.ID)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
					}
				}

		}
		if len(block.PrevHash) == 0{
			break
		}
	}
	return UTXO
}



func (bc *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := bc.Iterator()

	for {
		block := iter.Next()
		for _, tx := range block.Transactions{
			if bytes.Compare(tx.ID, ID) == 0{
				return *tx, nil
			}
		}

		if len(block.PrevHash) == 0{
			break
		}
	}
	return Transaction{}, errors.New("Transaction does not exist")

}

func (bc *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey){
	prevTXs := make(map[string]Transaction)
	for _, in := range tx.Inputs{
		prevTX, err := bc.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}
	tx.Sign(privKey, prevTXs)
}


func (bc *BlockChain) VerifyTransaction(tx *Transaction) bool {

	if tx.IsCoinbase(){
		return true
	}
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs{
		prevTX, err := bc.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}
	return tx.Verify(prevTXs)
}

func retry(dir string, originalOpts badger.Options) (*badger.DB, error){
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil{
		return nil, fmt.Errorf("Removing Lock : %s", err)

	}
	retryOpts := originalOpts
	retryOpts.Truncate = true
	db, err := badger.Open(retryOpts)
	return db, err
} 

func openDB(dir string, opts badger.Options) (*badger.DB, error){
	if db, err := badger.Open(opts); err != nil{
		if strings.Contains(err.Error(), "LOCK"){
			if db, err := retry(dir, opts); err == nil{
				log.Println("Database unlocked, value log truncated")
				return db, nil
			}
			log.Println("could not unlock database:", err)
		}
		return nil, err
	}else{
		return db, nil
	}
}