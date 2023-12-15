package exporter

import (
	"context"
	"fmt"
	"time"

	tmrpc "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
)

type ChainStatus struct {
	status *coretypes.ResultStatus
}

func NewChainStatus(config *ServiceConfig) (ChainStatus, error) {
	client, err := tmrpc.New(config.TendermintRPC, "/websocket")
	if err != nil {
		return ChainStatus{}, err
	}

	status, err := client.Status(context.Background())
	if err != nil {
		return ChainStatus{}, err
	}

	return ChainStatus{
		status: status,
	}, nil
}

func (cs ChainStatus) SyncInfo() coretypes.SyncInfo {
	return cs.status.SyncInfo
}

func (cs ChainStatus) LatestBlockTime() time.Time {
	return cs.SyncInfo().LatestBlockTime
}

func (cs ChainStatus) AvgBlockTIme() float64 {
	info := cs.SyncInfo()
	diffHeight := float64(info.LatestBlockHeight - info.EarliestBlockHeight)
	diffSeconds := float64(info.LatestBlockTime.Unix() - info.EarliestBlockTime.Unix())
	avgTime := diffSeconds / diffHeight

	return avgTime
}

func (cs ChainStatus) EstimateBlockTime(totalHeight int64) (time.Time, error) {
	latestBlockTime := cs.LatestBlockTime()
	avgBlockTime := cs.AvgBlockTIme()
	totalTime := int64(float64(totalHeight) * avgBlockTime)
	s := fmt.Sprintf("%ds", totalTime)

	duration, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, err
	}

	estimated := latestBlockTime.Add(duration)

	return estimated, nil
}
