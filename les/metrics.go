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

package les

import (
	"github.com/goola-team/goola/metrics"
	"github.com/goola-team/goola/p2p"
)

var (
	/*	propTxnInPacketsMeter     = metrics.NewMeter("goolabackend/prop/txns/in/packets")
		propTxnInTrafficMeter     = metrics.NewMeter("goolabackend/prop/txns/in/traffic")
		propTxnOutPacketsMeter    = metrics.NewMeter("goolabackend/prop/txns/out/packets")
		propTxnOutTrafficMeter    = metrics.NewMeter("goolabackend/prop/txns/out/traffic")
		propHashInPacketsMeter    = metrics.NewMeter("goolabackend/prop/hashes/in/packets")
		propHashInTrafficMeter    = metrics.NewMeter("goolabackend/prop/hashes/in/traffic")
		propHashOutPacketsMeter   = metrics.NewMeter("goolabackend/prop/hashes/out/packets")
		propHashOutTrafficMeter   = metrics.NewMeter("goolabackend/prop/hashes/out/traffic")
		propBlockInPacketsMeter   = metrics.NewMeter("goolabackend/prop/blocks/in/packets")
		propBlockInTrafficMeter   = metrics.NewMeter("goolabackend/prop/blocks/in/traffic")
		propBlockOutPacketsMeter  = metrics.NewMeter("goolabackend/prop/blocks/out/packets")
		propBlockOutTrafficMeter  = metrics.NewMeter("goolabackend/prop/blocks/out/traffic")
		reqHashInPacketsMeter     = metrics.NewMeter("goolabackend/req/hashes/in/packets")
		reqHashInTrafficMeter     = metrics.NewMeter("goolabackend/req/hashes/in/traffic")
		reqHashOutPacketsMeter    = metrics.NewMeter("goolabackend/req/hashes/out/packets")
		reqHashOutTrafficMeter    = metrics.NewMeter("goolabackend/req/hashes/out/traffic")
		reqBlockInPacketsMeter    = metrics.NewMeter("goolabackend/req/blocks/in/packets")
		reqBlockInTrafficMeter    = metrics.NewMeter("goolabackend/req/blocks/in/traffic")
		reqBlockOutPacketsMeter   = metrics.NewMeter("goolabackend/req/blocks/out/packets")
		reqBlockOutTrafficMeter   = metrics.NewMeter("goolabackend/req/blocks/out/traffic")
		reqHeaderInPacketsMeter   = metrics.NewMeter("goolabackend/req/headers/in/packets")
		reqHeaderInTrafficMeter   = metrics.NewMeter("goolabackend/req/headers/in/traffic")
		reqHeaderOutPacketsMeter  = metrics.NewMeter("goolabackend/req/headers/out/packets")
		reqHeaderOutTrafficMeter  = metrics.NewMeter("goolabackend/req/headers/out/traffic")
		reqBodyInPacketsMeter     = metrics.NewMeter("goolabackend/req/bodies/in/packets")
		reqBodyInTrafficMeter     = metrics.NewMeter("goolabackend/req/bodies/in/traffic")
		reqBodyOutPacketsMeter    = metrics.NewMeter("goolabackend/req/bodies/out/packets")
		reqBodyOutTrafficMeter    = metrics.NewMeter("goolabackend/req/bodies/out/traffic")
		reqStateInPacketsMeter    = metrics.NewMeter("goolabackend/req/states/in/packets")
		reqStateInTrafficMeter    = metrics.NewMeter("goolabackend/req/states/in/traffic")
		reqStateOutPacketsMeter   = metrics.NewMeter("goolabackend/req/states/out/packets")
		reqStateOutTrafficMeter   = metrics.NewMeter("goolabackend/req/states/out/traffic")
		reqReceiptInPacketsMeter  = metrics.NewMeter("goolabackend/req/receipts/in/packets")
		reqReceiptInTrafficMeter  = metrics.NewMeter("goolabackend/req/receipts/in/traffic")
		reqReceiptOutPacketsMeter = metrics.NewMeter("goolabackend/req/receipts/out/packets")
		reqReceiptOutTrafficMeter = metrics.NewMeter("goolabackend/req/receipts/out/traffic")*/
	miscInPacketsMeter  = metrics.NewMeter("les/misc/in/packets")
	miscInTrafficMeter  = metrics.NewMeter("les/misc/in/traffic")
	miscOutPacketsMeter = metrics.NewMeter("les/misc/out/packets")
	miscOutTrafficMeter = metrics.NewMeter("les/misc/out/traffic")
)

// meteredMsgReadWriter is a wrapper around a p2p.MsgReadWriter, capable of
// accumulating the above defined metrics based on the data stream contents.
type meteredMsgReadWriter struct {
	p2p.MsgReadWriter     // Wrapped message stream to meter
	version           int // Protocol version to select correct meters
}

// newMeteredMsgWriter wraps a p2p MsgReadWriter with metering support. If the
// metrics system is disabled, this function returns the original object.
func newMeteredMsgWriter(rw p2p.MsgReadWriter) p2p.MsgReadWriter {
	if !metrics.Enabled {
		return rw
	}
	return &meteredMsgReadWriter{MsgReadWriter: rw}
}

// Init sets the protocol version used by the stream to know which meters to
// increment in case of overlapping message ids between protocol versions.
func (rw *meteredMsgReadWriter) Init(version int) {
	rw.version = version
}

func (rw *meteredMsgReadWriter) ReadMsg() (p2p.Msg, error) {
	// Read the message and short circuit in case of an error
	msg, err := rw.MsgReadWriter.ReadMsg()
	if err != nil {
		return msg, err
	}
	// Account for the data traffic
	packets, traffic := miscInPacketsMeter, miscInTrafficMeter
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	return msg, err
}

func (rw *meteredMsgReadWriter) WriteMsg(msg p2p.Msg) error {
	// Account for the data traffic
	packets, traffic := miscOutPacketsMeter, miscOutTrafficMeter
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	// Send the packet to the p2p layer
	return rw.MsgReadWriter.WriteMsg(msg)
}
