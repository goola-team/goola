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

// Contains the metrics collected by the downloader.

package downloader

import (
	"github.com/goola-team/goola/metrics"
)

var (
	headerInMeter      = metrics.NewMeter("goolabackend/downloader/headers/in")
	headerReqTimer     = metrics.NewTimer("goolabackend/downloader/headers/req")
	headerDropMeter    = metrics.NewMeter("goolabackend/downloader/headers/drop")
	headerTimeoutMeter = metrics.NewMeter("goolabackend/downloader/headers/timeout")

	bodyInMeter      = metrics.NewMeter("goolabackend/downloader/bodies/in")
	bodyReqTimer     = metrics.NewTimer("goolabackend/downloader/bodies/req")
	bodyDropMeter    = metrics.NewMeter("goolabackend/downloader/bodies/drop")
	bodyTimeoutMeter = metrics.NewMeter("goolabackend/downloader/bodies/timeout")

	receiptInMeter      = metrics.NewMeter("goolabackend/downloader/receipts/in")
	receiptReqTimer     = metrics.NewTimer("goolabackend/downloader/receipts/req")
	receiptDropMeter    = metrics.NewMeter("goolabackend/downloader/receipts/drop")
	receiptTimeoutMeter = metrics.NewMeter("goolabackend/downloader/receipts/timeout")

	stateInMeter   = metrics.NewMeter("goolabackend/downloader/states/in")
	stateDropMeter = metrics.NewMeter("goolabackend/downloader/states/drop")
)
