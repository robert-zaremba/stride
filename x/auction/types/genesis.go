package types

import (
"fmt"
)

// DefaultIndex is the default global index
const DefaultIndex uint64 = 1

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{
	    AuctionPoolList: []AuctionPool{},
// this line is used by starport scaffolding # genesis/types/default
	    Params:	DefaultParams(),
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
    // Check for duplicated ID in auctionPool
auctionPoolIdMap := make(map[uint64]bool)
auctionPoolCount := gs.GetAuctionPoolCount()
for _, elem := range gs.AuctionPoolList {
	if _, ok := auctionPoolIdMap[elem.Id]; ok {
		return fmt.Errorf("duplicated id for auctionPool")
	}
	if elem.Id >= auctionPoolCount {
		return fmt.Errorf("auctionPool id should be lower or equal than the last id")
	}
	auctionPoolIdMap[elem.Id] = true
}
// this line is used by starport scaffolding # genesis/types/validate

	return gs.Params.Validate()
}