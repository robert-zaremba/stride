package keeper

import (
	"context"
	"encoding/json"
	"time"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	icatypes "github.com/cosmos/ibc-go/v5/modules/apps/27-interchain-accounts/types"
	ibctmtypes "github.com/cosmos/ibc-go/v5/modules/light-clients/07-tendermint/types"

	"github.com/Stride-Labs/stride/v5/x/icaoracle/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// Adds a new oracle as a destination for metric updates
// Registers a new ICA account along this connection
func (k msgServer) AddOracle(goCtx context.Context, msg *types.MsgAddOracle) (*types.MsgAddOracleResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Grab the connection and confirm it exists
	controllerConnectionId := msg.ConnectionId
	connectionEnd, found := k.IBCKeeper.ConnectionKeeper.GetConnection(ctx, controllerConnectionId)
	if !found {
		return nil, errorsmod.Wrapf(sdkerrors.ErrNotFound, "connection (%s) not found", controllerConnectionId)
	}

	// Get chain id from the connection
	clientState, found := k.ICACallbacksKeeper.IBCKeeper.ClientKeeper.GetClientState(ctx, connectionEnd.ClientId)
	if !found {
		return nil, errorsmod.Wrapf(sdkerrors.ErrNotFound, "client (%s) not found", connectionEnd.ClientId)
	}
	client, ok := clientState.(*ibctmtypes.ClientState)
	if !ok {
		return nil, types.ErrClientStateNotTendermint
	}
	chainId := client.ChainId

	// Confirm oracle was not already created
	_, found = k.GetOracle(ctx, chainId)
	if found {
		return nil, types.ErrOracleAlreadyExists
	}

	// Create the oracle struct, marked as inactive
	oracle := types.Oracle{
		ChainId:      chainId,
		ConnectionId: controllerConnectionId,
		Active:       false,
	}
	k.SetOracle(ctx, oracle)

	// Get the corresponding connection on the host
	hostConnectionId := connectionEnd.Counterparty.ConnectionId
	if hostConnectionId == "" {
		return nil, types.ErrHostConnectionNotFound
	}

	// Register the oracle interchain account
	appVersion := string(icatypes.ModuleCdc.MustMarshalJSON(&icatypes.Metadata{
		Version:                icatypes.Version,
		ControllerConnectionId: controllerConnectionId,
		HostConnectionId:       hostConnectionId,
		Encoding:               icatypes.EncodingProtobuf,
		TxType:                 icatypes.TxTypeSDKMultiMsg,
	}))

	owner := types.FormatICAAccountOwner(chainId, types.ICAAccountType_Oracle)
	if err := k.ICAControllerKeeper.RegisterInterchainAccount(ctx, controllerConnectionId, owner, appVersion); err != nil {
		return nil, errorsmod.Wrapf(err, "unable to register oracle interchain account")
	}

	return &types.MsgAddOracleResponse{}, nil
}

// Instantiates the oracle cosmwasm contract
func (k msgServer) InstantiateOracle(goCtx context.Context, msg *types.MsgInstantiateOracle) (*types.MsgInstantiateOracleResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Confirm the oracle has already been added, but has not yet been instantiated
	oracle, found := k.GetOracle(ctx, msg.OracleChainId)
	if !found {
		return nil, types.ErrOracleNotFound
	}
	if oracle.ContractAddress != "" {
		return nil, types.ErrOracleAlreadyInstantiated
	}

	// Confirm the oracle ICA was registered
	if err := oracle.ValidateICASetup(); err != nil {
		return nil, err
	}

	// Build the contract-specific instantiation message
	contractMsg := types.MsgInstantiateOracleContract{
		AdminAddress: oracle.IcaAddress,
	}
	contractMsgBz, err := json.Marshal(contractMsg)
	if err != nil {
		return nil, errorsmod.Wrapf(err, "unable to marshal instantiate oracle contract")
	}

	// Build the ICA message to instantiate the contract
	msgs := []sdk.Msg{&types.MsgInstantiateContract{
		Sender: oracle.IcaAddress,
		Admin:  oracle.IcaAddress,
		CodeID: msg.ContractCodeId,
		Label:  "Stride ICA Oracle",
		Msg:    contractMsgBz,
	}}

	// Submit the ICA with a 1 day timeout
	// The timeout time here is arbitrary, but 1 day gives enough time to manually relay the packet if it gets stuck
	timeout := uint64(ctx.BlockTime().UnixNano() + (time.Hour * 24).Nanoseconds())

	// Submit the ICA
	callbackArgs := types.InstantiateOracleCallback{
		OracleChainId: oracle.ChainId,
	}
	icaTx := types.ICATx{
		ConnectionId: oracle.ConnectionId,
		ChannelId:    oracle.ChannelId,
		PortId:       oracle.PortId,
		Messages:     msgs,
		Timeout:      timeout,
		CallbackArgs: &callbackArgs,
		CallbackId:   ICACallbackID_InstantiateOracle,
	}
	if err := k.SubmitICATx(ctx, icaTx); err != nil {
		return nil, errorsmod.Wrapf(err, "unable to submit instantiate oracle contract ICA")
	}

	return &types.MsgInstantiateOracleResponse{}, nil
}

// Creates a new ICA channel and restores the oracle ICA account after a channel closer
func (k msgServer) RestoreOracleICA(goCtx context.Context, msg *types.MsgRestoreOracleICA) (*types.MsgRestoreOracleICAResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Confirm the oracle exists and has already had has already had an ICA registered
	oracle, found := k.GetOracle(ctx, msg.OracleChainId)
	if !found {
		return nil, types.ErrOracleNotFound
	}
	if err := oracle.ValidateICASetup(); err != nil {
		return nil, errorsmod.Wrapf(err, "the oracle (%s) has never had an registered ICA", oracle.ChainId)
	}

	// Grab the connectionEnd for the counterparty connection
	connectionEnd, found := k.IBCKeeper.ConnectionKeeper.GetConnection(ctx, oracle.ConnectionId)
	if !found {
		return nil, errorsmod.Wrapf(sdkerrors.ErrNotFound, "connection (%s) not found", oracle.ConnectionId)
	}
	hostConnectionId := connectionEnd.Counterparty.ConnectionId

	// Only allow restoring an ICA if the account already exists
	owner := types.FormatICAAccountOwner(oracle.ChainId, types.ICAAccountType_Oracle)
	portId, err := icatypes.NewControllerPortID(owner)
	if err != nil {
		return nil, errorsmod.Wrapf(err, "unable to build portId from owner (%s)", owner)
	}
	_, exists := k.ICAControllerKeeper.GetInterchainAccountAddress(ctx, oracle.ConnectionId, portId)
	if !exists {
		return nil, errorsmod.Wrapf(types.ErrICAAccountDoesNotExist, "cannot find ICA account for connection (%s) and port (%s)", oracle.ConnectionId, portId)
	}

	// Call register ICA again to restore the account
	appVersion := string(icatypes.ModuleCdc.MustMarshalJSON(&icatypes.Metadata{
		Version:                icatypes.Version,
		ControllerConnectionId: oracle.ConnectionId,
		HostConnectionId:       hostConnectionId,
		Encoding:               icatypes.EncodingProtobuf,
		TxType:                 icatypes.TxTypeSDKMultiMsg,
	}))
	if err := k.ICAControllerKeeper.RegisterInterchainAccount(ctx, oracle.ConnectionId, owner, appVersion); err != nil {
		return nil, errorsmod.Wrapf(err, "unable to register oracle interchain account")
	}

	// Delete all pending ICAs along the old channel
	for _, pendingMetricUpdate := range k.GetAllPendingMetricUpdates(ctx) {
		if pendingMetricUpdate.OracleChainId == oracle.ChainId {
			k.SetMetricUpdateComplete(ctx, pendingMetricUpdate.Metric.Key, oracle.ChainId)
		}
	}

	return &types.MsgRestoreOracleICAResponse{}, nil
}