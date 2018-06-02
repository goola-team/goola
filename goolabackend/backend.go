// Copyright 2014 The go-ethereum Authors
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

// Package goola implements the Goola protocol.
package goolabackend

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/goola-team/goola/accounts"
	"github.com/goola-team/goola/common"
	"github.com/goola-team/goola/common/hexutil"
	"github.com/goola-team/goola/consensus"
	"github.com/goola-team/goola/consensus/clique"
	"github.com/goola-team/goola/core"
	"github.com/goola-team/goola/core/bloombits"
	"github.com/goola-team/goola/core/types"
	"github.com/goola-team/goola/core/vm"
	"github.com/goola-team/goola/goolabackend/downloader"
	"github.com/goola-team/goola/goolabackend/filters"
	"github.com/goola-team/goola/goolabackend/gasprice"
	"github.com/goola-team/goola/gooladb"
	"github.com/goola-team/goola/event"
	"github.com/goola-team/goola/internal/ethapi"
	"github.com/goola-team/goola/log"
	"github.com/goola-team/goola/miner"
	"github.com/goola-team/goola/node"
	"github.com/goola-team/goola/p2p"
	"github.com/goola-team/goola/params"
	"github.com/goola-team/goola/rlp"
	"github.com/goola-team/goola/rpc"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
	SetBloomBitsIndexer(bbIndexer *core.ChainIndexer)
}

// FullGoola implements the FullGoola full node service.
type FullGoola struct {
	config      *Config
	chainConfig *params.ChainConfig

	// Channel for shutting down the service
	shutdownChan  chan bool    // Channel for shutting down the FullGoola
	stopDbUpgrade func() error // stop chain db sequential key upgrade

	// Handlers
	txPool          *core.TxPool
	blockchain      *core.BlockChain
	protocolManager *ProtocolManager
	lesServer       LesServer

	// DB interfaces
	chainDb gooladb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	ApiBackend *GoolaApiBackend

	miner     *miner.Miner
	gasPrice  *big.Int
	etherbase common.Address

	networkId     uint64
	netRPCService *ethapi.PublicNetAPI

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and goolase)
}

func (fullGoola *FullGoola) AddLesServer(ls LesServer) {
	fullGoola.lesServer = ls
	ls.SetBloomBitsIndexer(fullGoola.bloomIndexer)
}

// New creates a new FullGoola object (including the
// initialisation of the common FullGoola object)
func New(ctx *node.ServiceContext, config *Config) (*FullGoola, error) {
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run goolabackend.Ethereum in light sync mode, use les.LightEthereum")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	chainDb, err := CreateDB(ctx, config, "chaindata")
	if err != nil {
		return nil, err
	}
	stopDbUpgrade := upgradeDeduplicateData(chainDb)
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	fullGoola := &FullGoola{
		config:         config,
		chainDb:        chainDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, chainConfig, chainDb),
		shutdownChan:   make(chan bool),
		stopDbUpgrade:  stopDbUpgrade,
		networkId:      config.NetworkId,
		gasPrice:       config.GasPrice,
		etherbase:      config.Etherbase,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, params.BloomBitsBlocks),
	}

	log.Info("Initialising Goola protocol", "versions", ProtocolVersions, "network", config.NetworkId)

	if !config.SkipBcVersionCheck {
		bcVersion := core.GetBlockChainVersion(chainDb)
		if bcVersion != core.BlockChainVersion && bcVersion != 0 {
			return nil, fmt.Errorf("Blockchain DB version mismatch (%d / %d). Run goola upgradedb.\n", bcVersion, core.BlockChainVersion)
		}
		core.WriteBlockChainVersion(chainDb, core.BlockChainVersion)
	}
	var (
		vmConfig    = vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
		cacheConfig = &core.CacheConfig{Disabled: config.NoPruning, TrieNodeLimit: config.TrieCache, TrieTimeLimit: config.TrieTimeout}
	)
	fullGoola.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, fullGoola.chainConfig, fullGoola.engine, vmConfig)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		fullGoola.blockchain.SetHead(compat.RewindTo)
		core.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	fullGoola.bloomIndexer.Start(fullGoola.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = ctx.ResolvePath(config.TxPool.Journal)
	}
	fullGoola.txPool = core.NewTxPool(config.TxPool, fullGoola.chainConfig, fullGoola.blockchain)

	if fullGoola.protocolManager, err = NewProtocolManager(fullGoola.chainConfig, config.SyncMode, config.NetworkId, fullGoola.eventMux, fullGoola.txPool, fullGoola.engine, fullGoola.blockchain, chainDb); err != nil {
		return nil, err
	}
	fullGoola.miner = miner.New(fullGoola, fullGoola.chainConfig, fullGoola.EventMux(), fullGoola.engine)
	fullGoola.miner.SetExtra(makeExtraData(config.ExtraData))

	fullGoola.ApiBackend = &GoolaApiBackend{fullGoola, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	fullGoola.ApiBackend.gpo = gasprice.NewOracle(fullGoola.ApiBackend, gpoParams)

	return fullGoola, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"goola",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// CreateDB creates the chain database.
func CreateDB(ctx *node.ServiceContext, config *Config, name string) (gooladb.Database, error) {
	db, err := ctx.OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*gooladb.LDBDatabase); ok {
		db.Meter("goolabackend/db/chaindata/")
	}
	return db, nil
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Goola service
func CreateConsensusEngine(ctx *node.ServiceContext, chainConfig *params.ChainConfig, db gooladb.Database) consensus.Engine {
	// If proof-of-authority is requested, set it up
		return clique.New(chainConfig.Clique, db)
}

// APIs returns the collection of RPC services the Goola package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (fullGoola *FullGoola) APIs() []rpc.API {
	apis := ethapi.GetAPIs(fullGoola.ApiBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, fullGoola.engine.APIs(fullGoola.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "goolabackend",
			Version:   "1.0",
			Service:   NewPublicEthereumAPI(fullGoola),
			Public:    true,
		}, {
			Namespace: "goolabackend",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(fullGoola),
			Public:    true,
		}, {
			Namespace: "goolabackend",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(fullGoola.protocolManager.downloader, fullGoola.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(fullGoola),
			Public:    false,
		}, {
			Namespace: "goolabackend",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(fullGoola.ApiBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(fullGoola),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(fullGoola),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(fullGoola.chainConfig, fullGoola),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   fullGoola.netRPCService,
			Public:    true,
		},
	}...)
}

func (fullGoola *FullGoola) ResetWithGenesisBlock(gb *types.Block) {
	fullGoola.blockchain.ResetWithGenesisBlock(gb)
}

func (fullGoola *FullGoola) Goolase() (eb common.Address, err error) {
	fullGoola.lock.RLock()
	goolase := fullGoola.etherbase
	fullGoola.lock.RUnlock()

	if goolase != (common.Address{}) {
		return goolase, nil
	}
	if wallets := fullGoola.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			goolase := accounts[0].Address

			fullGoola.lock.Lock()
			fullGoola.etherbase = goolase
			fullGoola.lock.Unlock()

			log.Info("Goolase automatically configured", "address", goolase)
			return goolase, nil
		}
	}
	return common.Address{}, fmt.Errorf("goolase must be explicitly specified")
}

// set in js console via admin interface or wrapper from cli flags
func (fullGoola *FullGoola) SetEtherbase(goolase common.Address) {
	fullGoola.lock.Lock()
	fullGoola.etherbase = goolase
	fullGoola.lock.Unlock()

	fullGoola.miner.SetEtherbase(goolase)
}

func (fullGoola *FullGoola) StartMining(local bool) error {
	eb, err := fullGoola.Goolase()
	if err != nil {
		log.Error("Cannot start mining without goolase", "err", err)
		return fmt.Errorf("etherbase missing: %v", err)
	}
	if clique, ok := fullGoola.engine.(*clique.Clique); ok {
		wallet, err := fullGoola.accountManager.Find(accounts.Account{Address: eb})
		if wallet == nil || err != nil {
			log.Error("Goolase account unavailable locally", "err", err)
			return fmt.Errorf("signer missing: %v", err)
		}
		clique.Authorize(eb, wallet.SignHash)
	}
	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so noone will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&fullGoola.protocolManager.acceptTxs, 1)
	}
	go fullGoola.miner.Start(eb)
	return nil
}

func (fullGoola *FullGoola) StopMining()         { fullGoola.miner.Stop() }
func (fullGoola *FullGoola) IsMining() bool      { return fullGoola.miner.Mining() }
func (fullGoola *FullGoola) Miner() *miner.Miner { return fullGoola.miner }

func (fullGoola *FullGoola) AccountManager() *accounts.Manager  { return fullGoola.accountManager }
func (fullGoola *FullGoola) BlockChain() *core.BlockChain       { return fullGoola.blockchain }
func (fullGoola *FullGoola) TxPool() *core.TxPool               { return fullGoola.txPool }
func (fullGoola *FullGoola) EventMux() *event.TypeMux           { return fullGoola.eventMux }
func (fullGoola *FullGoola) Engine() consensus.Engine           { return fullGoola.engine }
func (fullGoola *FullGoola) ChainDb() gooladb.Database          { return fullGoola.chainDb }
func (fullGoola *FullGoola) IsListening() bool                  { return true } // Always listening
func (fullGoola *FullGoola) EthVersion() int                    { return int(fullGoola.protocolManager.SubProtocols[0].Version) }
func (fullGoola *FullGoola) NetVersion() uint64                 { return fullGoola.networkId }
func (fullGoola *FullGoola) Downloader() *downloader.Downloader { return fullGoola.protocolManager.downloader }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (fullGoola *FullGoola) Protocols() []p2p.Protocol {
	if fullGoola.lesServer == nil {
		return fullGoola.protocolManager.SubProtocols
	}
	return append(fullGoola.protocolManager.SubProtocols, fullGoola.lesServer.Protocols()...)
}

// Start implements node.Service, starting all internal goroutines needed by the
// Goola protocol implementation.
func (fullGoola *FullGoola) Start(srvr *p2p.Server) error {
	// Start the bloom bits servicing goroutines
	fullGoola.startBloomHandlers()

	// Start the RPC service
	fullGoola.netRPCService = ethapi.NewPublicNetAPI(srvr, fullGoola.NetVersion())

	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers
	if fullGoola.config.LightServ > 0 {
		if fullGoola.config.LightPeers >= srvr.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", fullGoola.config.LightPeers, srvr.MaxPeers)
		}
		maxPeers -= fullGoola.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	fullGoola.protocolManager.Start(maxPeers)
	if fullGoola.lesServer != nil {
		fullGoola.lesServer.Start(srvr)
	}
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Goola protocol.
func (fullGoola *FullGoola) Stop() error {
	if fullGoola.stopDbUpgrade != nil {
		fullGoola.stopDbUpgrade()
	}
	fullGoola.bloomIndexer.Close()
	fullGoola.blockchain.Stop()
	fullGoola.protocolManager.Stop()
	if fullGoola.lesServer != nil {
		fullGoola.lesServer.Stop()
	}
	fullGoola.txPool.Stop()
	fullGoola.miner.Stop()
	fullGoola.eventMux.Stop()

	fullGoola.chainDb.Close()
	close(fullGoola.shutdownChan)

	return nil
}
