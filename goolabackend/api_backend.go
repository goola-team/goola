// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package goolabackend

import (
	"context"
	"math/big"

	"github.com/goola-team/goola/accounts"
	"github.com/goola-team/goola/common"
	"github.com/goola-team/goola/common/math"
	"github.com/goola-team/goola/core"
	"github.com/goola-team/goola/core/bloombits"
	"github.com/goola-team/goola/core/state"
	"github.com/goola-team/goola/core/types"
	"github.com/goola-team/goola/core/vm"
	"github.com/goola-team/goola/goolabackend/downloader"
	"github.com/goola-team/goola/goolabackend/gasprice"
	"github.com/goola-team/goola/gooladb"
	"github.com/goola-team/goola/event"
	"github.com/goola-team/goola/params"
	"github.com/goola-team/goola/rpc"
)

// GoolaApiBackend implements ethapi.Backend for full nodes
type GoolaApiBackend struct {
	goola *FullGoola
	gpo   *gasprice.Oracle
}

func (b *GoolaApiBackend) ChainConfig() *params.ChainConfig {
	return b.goola.chainConfig
}

func (b *GoolaApiBackend) CurrentBlock() *types.Block {
	return b.goola.blockchain.CurrentBlock()
}

func (b *GoolaApiBackend) SetHead(number uint64) {
	b.goola.protocolManager.downloader.Cancel()
	b.goola.blockchain.SetHead(number)
}

func (b *GoolaApiBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.goola.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.goola.blockchain.CurrentBlock().Header(), nil
	}
	return b.goola.blockchain.GetHeaderByNumber(uint64(blockNr)), nil
}

func (b *GoolaApiBackend) BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.goola.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.goola.blockchain.CurrentBlock(), nil
	}
	return b.goola.blockchain.GetBlockByNumber(uint64(blockNr)), nil
}

func (b *GoolaApiBackend) StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block, state := b.goola.miner.Pending()
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, blockNr)
	if header == nil || err != nil {
		return nil, nil, err
	}
	stateDb, err := b.goola.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *GoolaApiBackend) GetBlock(ctx context.Context, blockHash common.Hash) (*types.Block, error) {
	return b.goola.blockchain.GetBlockByHash(blockHash), nil
}

func (b *GoolaApiBackend) GetReceipts(ctx context.Context, blockHash common.Hash) (types.Receipts, error) {
	return core.GetBlockReceipts(b.goola.chainDb, blockHash, core.GetBlockNumber(b.goola.chainDb, blockHash)), nil
}


func (b *GoolaApiBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error) {
	state.SetBalance(msg.From(), math.MaxBig256)
	vmError := func() error { return nil }

	context := core.NewEVMContext(msg, header, b.goola.BlockChain(), nil)
	return vm.NewEVM(context, state, b.goola.chainConfig, vmCfg), vmError, nil
}

func (b *GoolaApiBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.goola.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *GoolaApiBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.goola.BlockChain().SubscribeChainEvent(ch)
}

func (b *GoolaApiBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.goola.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *GoolaApiBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return b.goola.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *GoolaApiBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.goola.BlockChain().SubscribeLogsEvent(ch)
}

func (b *GoolaApiBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.goola.txPool.AddLocal(signedTx)
}

func (b *GoolaApiBackend) GetPoolTransactions() (types.Transactions, error) {
	pending, err := b.goola.txPool.Pending()
	if err != nil {
		return nil, err
	}
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *GoolaApiBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.goola.txPool.Get(hash)
}

func (b *GoolaApiBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.goola.txPool.State().GetNonce(addr), nil
}

func (b *GoolaApiBackend) Stats() (pending int, queued int) {
	return b.goola.txPool.Stats()
}

func (b *GoolaApiBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.goola.TxPool().Content()
}

func (b *GoolaApiBackend) SubscribeTxPreEvent(ch chan<- core.TxPreEvent) event.Subscription {
	return b.goola.TxPool().SubscribeTxPreEvent(ch)
}

func (b *GoolaApiBackend) Downloader() *downloader.Downloader {
	return b.goola.Downloader()
}

func (b *GoolaApiBackend) ProtocolVersion() int {
	return b.goola.EthVersion()
}

func (b *GoolaApiBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *GoolaApiBackend) ChainDb() gooladb.Database {
	return b.goola.ChainDb()
}

func (b *GoolaApiBackend) EventMux() *event.TypeMux {
	return b.goola.EventMux()
}

func (b *GoolaApiBackend) AccountManager() *accounts.Manager {
	return b.goola.AccountManager()
}

func (b *GoolaApiBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.goola.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *GoolaApiBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.goola.bloomRequests)
	}
}
