// Copyright 2016 The go-ethereum Authors
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

// Package les implements the Light Goola Subprotocol.
package les

import (
	"fmt"
	"sync"
	"time"

	"github.com/goola-team/goola/accounts"
	"github.com/goola-team/goola/common"
	"github.com/goola-team/goola/consensus"
	"github.com/goola-team/goola/core"
	"github.com/goola-team/goola/core/bloombits"
	"github.com/goola-team/goola/core/types"
	"github.com/goola-team/goola/goolabackend"
	"github.com/goola-team/goola/goolabackend/downloader"
	"github.com/goola-team/goola/goolabackend/filters"
	"github.com/goola-team/goola/goolabackend/gasprice"
	"github.com/goola-team/goola/gooladb"
	"github.com/goola-team/goola/event"
	"github.com/goola-team/goola/internal/ethapi"
	"github.com/goola-team/goola/light"
	"github.com/goola-team/goola/log"
	"github.com/goola-team/goola/node"
	"github.com/goola-team/goola/p2p"
	"github.com/goola-team/goola/p2p/discv5"
	"github.com/goola-team/goola/params"
	rpc "github.com/goola-team/goola/rpc"
)

type LightGoola struct {
	config *goolabackend.Config

	odr         *LesOdr
	relay       *LesTxRelay
	chainConfig *params.ChainConfig
	// Channel for shutting down the service
	shutdownChan chan bool
	// Handlers
	peers           *peerSet
	txPool          *light.TxPool
	blockchain      *light.LightChain
	protocolManager *ProtocolManager
	serverPool      *serverPool
	reqDist         *requestDistributor
	retriever       *retrieveManager
	// DB interfaces
	chainDb gooladb.Database // Block chain database

	bloomRequests                              chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer, chtIndexer, bloomTrieIndexer *core.ChainIndexer

	ApiBackend *LesApiBackend

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	networkId     uint64
	netRPCService *ethapi.PublicNetAPI

	wg sync.WaitGroup
}

func New(ctx *node.ServiceContext, config *goolabackend.Config) (*LightGoola, error) {
	chainDb, err := goolabackend.CreateDB(ctx, config, "lightchaindata")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, isCompat := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !isCompat {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	peers := newPeerSet()
	quitSync := make(chan struct{})

	lightGoola := &LightGoola{
		config:           config,
		chainConfig:      chainConfig,
		chainDb:          chainDb,
		eventMux:         ctx.EventMux,
		peers:            peers,
		reqDist:          newRequestDistributor(peers, quitSync),
		accountManager:   ctx.AccountManager,
		engine:           goolabackend.CreateConsensusEngine(ctx, chainConfig, chainDb),
		shutdownChan:     make(chan bool),
		networkId:        config.NetworkId,
		bloomRequests:    make(chan chan *bloombits.Retrieval),
		bloomIndexer:     goolabackend.NewBloomIndexer(chainDb, light.BloomTrieFrequency),
		chtIndexer:       light.NewChtIndexer(chainDb, true),
		bloomTrieIndexer: light.NewBloomTrieIndexer(chainDb, true),
	}

	lightGoola.relay = NewLesTxRelay(peers, lightGoola.reqDist)
	lightGoola.serverPool = newServerPool(chainDb, quitSync, &lightGoola.wg)
	lightGoola.retriever = newRetrieveManager(peers, lightGoola.reqDist, lightGoola.serverPool)
	lightGoola.odr = NewLesOdr(chainDb, lightGoola.chtIndexer, lightGoola.bloomTrieIndexer, lightGoola.bloomIndexer, lightGoola.retriever)
	if lightGoola.blockchain, err = light.NewLightChain(lightGoola.odr, lightGoola.chainConfig, lightGoola.engine); err != nil {
		return nil, err
	}
	lightGoola.bloomIndexer.Start(lightGoola.blockchain)
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		lightGoola.blockchain.SetHead(compat.RewindTo)
		core.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}

	lightGoola.txPool = light.NewTxPool(lightGoola.chainConfig, lightGoola.blockchain, lightGoola.relay)
	if lightGoola.protocolManager, err = NewProtocolManager(lightGoola.chainConfig, true, ClientProtocolVersions, config.NetworkId, lightGoola.eventMux, lightGoola.engine, lightGoola.peers, lightGoola.blockchain, nil, chainDb, lightGoola.odr, lightGoola.relay, quitSync, &lightGoola.wg); err != nil {
		return nil, err
	}
	lightGoola.ApiBackend = &LesApiBackend{lightGoola, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	lightGoola.ApiBackend.gpo = gasprice.NewOracle(lightGoola.ApiBackend, gpoParams)
	return lightGoola, nil
}

func lesTopic(genesisHash common.Hash, protocolVersion uint) discv5.Topic {
	var name string
	switch protocolVersion {
	case lpv1:
		name = "LES"
	case lpv2:
		name = "LES2"
	default:
		panic(nil)
	}
	return discv5.Topic(name + "@" + common.Bytes2Hex(genesisHash.Bytes()[0:8]))
}

type LightDummyAPI struct{}

// Goolase is the address that mining rewards will be send to
func (lightGoola *LightGoola) Goolase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("not supported")
}

// Coinbase is the address that mining rewards will be send to (alias for Goolase)
func (lightGoola *LightGoola) Coinbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("not supported")
}


// Mining returns an indication if this node is currently mining.
func (lightGoola *LightGoola) Mining() bool {
	return false
}

// APIs returns the collection of RPC services the Goola package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (lightGoola *LightGoola) APIs() []rpc.API {
	return append(ethapi.GetAPIs(lightGoola.ApiBackend), []rpc.API{
		{
			Namespace: "goolabackend",
			Version:   "1.0",
			Service:   &LightDummyAPI{},
			Public:    true,
		}, {
			Namespace: "goolabackend",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(lightGoola.protocolManager.downloader, lightGoola.eventMux),
			Public:    true,
		}, {
			Namespace: "goolabackend",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(lightGoola.ApiBackend, true),
			Public:    true,
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   lightGoola.netRPCService,
			Public:    true,
		},
	}...)
}

func (lightGoola *LightGoola) ResetWithGenesisBlock(gb *types.Block) {
	lightGoola.blockchain.ResetWithGenesisBlock(gb)
}

func (lightGoola *LightGoola) BlockChain() *light.LightChain      { return lightGoola.blockchain }
func (lightGoola *LightGoola) TxPool() *light.TxPool              { return lightGoola.txPool }
func (lightGoola *LightGoola) Engine() consensus.Engine           { return lightGoola.engine }
func (lightGoola *LightGoola) LesVersion() int                    { return int(lightGoola.protocolManager.SubProtocols[0].Version) }
func (lightGoola *LightGoola) Downloader() *downloader.Downloader { return lightGoola.protocolManager.downloader }
func (lightGoola *LightGoola) EventMux() *event.TypeMux           { return lightGoola.eventMux }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (lightGoola *LightGoola) Protocols() []p2p.Protocol {
	return lightGoola.protocolManager.SubProtocols
}

// Start implements node.Service, starting all internal goroutines needed by the
// Goola protocol implementation.
func (lightGoola *LightGoola) Start(srvr *p2p.Server) error {
	lightGoola.startBloomHandlers()
	log.Warn("Light client mode is an experimental feature")
	lightGoola.netRPCService = ethapi.NewPublicNetAPI(srvr, lightGoola.networkId)
	// clients are searching for the first advertised protocol in the list
	protocolVersion := AdvertiseProtocolVersions[0]
	lightGoola.serverPool.start(srvr, lesTopic(lightGoola.blockchain.Genesis().Hash(), protocolVersion))
	lightGoola.protocolManager.Start(lightGoola.config.LightPeers)
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Goola protocol.
func (lightGoola *LightGoola) Stop() error {
	lightGoola.odr.Stop()
	if lightGoola.bloomIndexer != nil {
		lightGoola.bloomIndexer.Close()
	}
	if lightGoola.chtIndexer != nil {
		lightGoola.chtIndexer.Close()
	}
	if lightGoola.bloomTrieIndexer != nil {
		lightGoola.bloomTrieIndexer.Close()
	}
	lightGoola.blockchain.Stop()
	lightGoola.protocolManager.Stop()
	lightGoola.txPool.Stop()

	lightGoola.eventMux.Stop()

	time.Sleep(time.Millisecond * 200)
	lightGoola.chainDb.Close()
	close(lightGoola.shutdownChan)

	return nil
}
