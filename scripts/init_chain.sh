#!/bin/bash

set -eu 
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

source $SCRIPT_DIR/vars.sh

CHAIN_ID="$1"

# first get the chain specific variable names
CMD_VAR=${CHAIN_ID}_CMD
DENOM_VAR=${CHAIN_ID}_DENOM
RPC_PORT_VAR=${CHAIN_ID}_RPC_PORT
NUM_NODES_VAR=${CHAIN_ID}_NUM_NODES
NODE_PREFIX_VAR=${CHAIN_ID}_NODE_PREFIX
VAL_PREFIX_VAR=${CHAIN_ID}_VAL_PREFIX
VAL_MNEMONICS_VAR=${CHAIN_ID}_VAL_MNEMONICS

REV_ACCT_VAR=${CHAIN_ID}_REV_ACCT
REV_MNEMONIC_VAR=${CHAIN_ID}_REV_MNEMONIC
HERMES_ACCT_VAR=HERMES_${CHAIN_ID}_ACCT
HERMES_MNEMONIC_VAR=HERMES_${CHAIN_ID}_MNEMONIC
ICQ_ACCT_VAR=ICQ_${CHAIN_ID}_ACCT
ICQ_MNEMONIC_VAR=ICQ_${CHAIN_ID}_MNEMONIC

# then get the actual values of those variables
CMD=${!CMD_VAR}
DENOM=${!DENOM_VAR}
RPC_PORT=${!RPC_PORT_VAR}
NUM_NODES=${!NUM_NODES_VAR}
NODE_PREFIX=${!NODE_PREFIX_VAR}
VAL_PREFIX=${!VAL_PREFIX_VAR}
IFS=',' read -r -a VAL_MNEMONICS <<< "${!VAL_MNEMONICS_VAR}"

HERMES_ACCT=${!HERMES_ACCT_VAR}
HERMES_MNEMONIC=${!HERMES_MNEMONIC_VAR}
ICQ_ACCT=${!ICQ_ACCT_VAR}
ICQ_MNEMONIC=${!ICQ_MNEMONIC_VAR}

set_stride_genesis() {
    genesis_config=$1

    # update params
    jq '.app_state.epochs.epochs[$epochIndex].duration = $epochLen' --arg epochLen $DAY_EPOCH_DURATION --argjson epochIndex $DAY_EPOCH_INDEX  $genesis_config > json.tmp && mv json.tmp $genesis_config
    jq '.app_state.epochs.epochs[$epochIndex].duration = $epochLen' --arg epochLen $STRIDE_EPOCH_DURATION --argjson epochIndex $STRIDE_EPOCH_INDEX $genesis_config > json.tmp && mv json.tmp $genesis_config
    jq '.app_state.staking.params.unbonding_time = $newVal' --arg newVal "$UNBONDING_TIME" $genesis_config > json.tmp && mv json.tmp $genesis_config
    jq '.app_state.gov.deposit_params.max_deposit_period = $newVal' --arg newVal "$MAX_DEPOSIT_PERIOD" $genesis_config > json.tmp && mv json.tmp $genesis_config
    jq '.app_state.gov.voting_params.voting_period = $newVal' --arg newVal "$VOTING_PERIOD" $genesis_config > json.tmp && mv json.tmp $genesis_config
}

set_host_genesis() {
    genesis_config=$1

    # Shorten unbonding period
    jq '.app_state.staking.params.unbonding_time = $newVal' --arg newVal "$UNBONDING_TIME" $genesis_config > json.tmp && mv json.tmp $genesis_config

    # Add interchain accounts to the genesis set
    jq "del(.app_state.interchain_accounts)" $genesis_config > json.tmp && mv json.tmp $genesis_config
    interchain_accts=$(cat $SCRIPT_DIR/config/ica.json)
    jq ".app_state += $interchain_accts" $genesis_config > json.tmp && mv json.tmp $genesis_config
}

MAIN_ID=1 # Node responsible for genesis and persistent_peers
MAIN_NODE_NAME=""
MAIN_NODE_CMD=""
MAIN_NODE_ID=""
MAIN_CONFIG=""
MAIN_GENESIS=""
echo 'Initializing gaia chain...'
for (( i=1; i <= $NUM_NODES; i++ )); do
    # Node names will be of the form: "stride-node1"
    node_name="${NODE_PREFIX}${i}"
    # Moniker is of the form: STRIDE_1
    moniker=$(printf "${NODE_PREFIX}_${i}" | awk '{ print toupper($0) }')

    # Create a state directory for the current node and initialize the chain
    mkdir -p $STATE/$node_name
    cmd="$CMD --home ${STATE}/$node_name"
    $cmd init $moniker --chain-id $CHAIN_ID --overwrite 2> /dev/null

    # Update node networking configuration 
    config_toml="${STATE}/${node_name}/config/config.toml"
    client_toml="${STATE}/${node_name}/config/client.toml"
    app_toml="${STATE}/${node_name}/config/app.toml"
    genesis_json="${STATE}/${node_name}/config/genesis.json"

    sed -i -E "s|cors_allowed_origins = \[\]|cors_allowed_origins = [\"\*\"]|g" $config_toml
    sed -i -E "s|127.0.0.1|0.0.0.0|g" $config_toml
    sed -i -E "s|timeout_commit = \"5s\"|timeout_commit = \"${BLOCK_TIME}\"|g" $config_toml
    sed -i -E "s|prometheus = false|prometheus = true|g" $config_toml

    sed -i -E "s|minimum-gas-prices = \".*\"|minimum-gas-prices = \"0${DENOM}\"|g" $app_toml
    sed -i -E '/\[api\]/,/^enable = .*$/ s/^enable = .*$/enable = true/' $app_toml
    sed -i -E 's|unsafe-cors = .*|unsafe-cors = true|g' $app_toml

    sed -i -E "s|chain-id = \"\"|chain-id = \"${CHAIN_ID}\"|g" $client_toml
    sed -i -E "s|keyring-backend = \"os\"|keyring-backend = \"test\"|g" $client_toml
    sed -i -E "s|node = \".*\"|node = \"tcp://localhost:$RPC_PORT\"|g" $client_toml

    sed -i -E "s|\"stake\"|\"${DENOM}\"|g" $genesis_json

    # Get the endpoint and node ID
    node_id=$($cmd tendermint show-node-id)@$node_name:$PEER_PORT
    echo "Node #$i ID: $node_id"

    # add a validator account
    val_acct="${VAL_PREFIX}${i}"
    val_mnemonic="${VAL_MNEMONICS[((i-1))]}"
    echo "$val_mnemonic" | $cmd keys add $val_acct --recover --keyring-backend=test 
    val_addr=$($cmd keys show $val_acct --keyring-backend test -a)
    # Add this account to the current node
    $cmd add-genesis-account ${val_addr} ${VAL_TOKENS}${DENOM}
    # actually set this account as a validator on the current node 
    $cmd gentx $val_acct ${STAKE_TOKENS}${DENOM} --chain-id $CHAIN_ID --keyring-backend test 2> /dev/null

    # Cleanup from seds
    rm -rf ${client_toml}-E
    rm -rf ${genesis_json}-E
    rm -rf ${app_toml}-E

    if [ $i -eq $MAIN_ID ]; then
        MAIN_NODE_NAME=$node_name
        MAIN_NODE_CMD=$cmd
        MAIN_NODE_ID=$node_id
        MAIN_CONFIG=$config_toml
        MAIN_GENESIS=$genesis_json
    else
        # also add this account and it's genesis tx to the main node
        $MAIN_NODE_CMD add-genesis-account ${val_addr} ${VAL_TOKENS}${DENOM}
        cp ${STATE}/${node_name}/config/gentx/*.json ${STATE}/${MAIN_NODE_NAME}/config/gentx/

        # and add each validator's keys to the first state directory
        echo "$val_mnemonic" | $MAIN_NODE_CMD keys add $val_acct --recover --keyring-backend=test 
    fi
done

# add Hermes and ICQ relayer accounts on Stride
echo "$HERMES_MNEMONIC" | $MAIN_NODE_CMD keys add $HERMES_ACCT --recover --keyring-backend=test 
echo "$ICQ_MNEMONIC" | $MAIN_NODE_CMD keys add $ICQ_ACCT --recover --keyring-backend=test
HERMES_ADDRESS=$($MAIN_NODE_CMD keys show $HERMES_ACCT --keyring-backend test -a)
ICQ_ADDRESS=$($MAIN_NODE_CMD keys show $ICQ_ACCT --keyring-backend test -a)

# give relayer accounts token balances
$MAIN_NODE_CMD add-genesis-account ${HERMES_ADDRESS} ${VAL_TOKENS}${DENOM}
$MAIN_NODE_CMD add-genesis-account ${ICQ_ADDRESS} ${VAL_TOKENS}${DENOM}

if [ "$CHAIN_ID" == "$STRIDE_CHAIN_ID" ]; then 
    # add the stride admin account
    echo "$STRIDE_ADMIN_MNEMONIC" | $MAIN_NODE_CMD keys add $STRIDE_ADMIN_ACCT --recover --keyring-backend=test
    STRIDE_ADMIN_ADDRESS=$($MAIN_NODE_CMD keys show $STRIDE_ADMIN_ACCT --keyring-backend test -a)
    $MAIN_NODE_CMD add-genesis-account ${STRIDE_ADMIN_ADDRESS} ${ADMIN_TOKENS}${DENOM}
else 
    # add a revenue account
    REV_ACCT=${!REV_ACCT_VAR}
    REV_MNEMONIC=${!REV_MNEMONIC_VAR}
    echo $REV_MNEMONIC | $MAIN_NODE_CMD keys add $REV_ACCT --recover --keyring-backend=test
fi

# now we process gentx txs on the main node
$MAIN_NODE_CMD collect-gentxs 2> /dev/null

# wipe out the persistent peers for the main node (these are incorrectly autogenerated for each validator during collect-gentxs)
sed -i -E "s|persistent_peers = .*|persistent_peers = \"\"|g" $MAIN_CONFIG

# update chian-specific genesis settings
if [ "$CHAIN_ID" == "$STRIDE_CHAIN_ID" ]; then 
    set_stride_genesis $MAIN_GENESIS
else
    set_host_genesis $MAIN_GENESIS
fi

# for all peer nodes....
for (( i=2; i <= $NUM_NODES; i++ )); do
    node_name="${NODE_PREFIX}${i}"
    config_toml="${STATE}/${node_name}/config/config.toml"
    genesis_json="${STATE}/${node_name}/config/genesis.json"

    # add the main node as a persistent peer
    sed -i -E "s|persistent_peers = .*|persistent_peers = \"${MAIN_NODE_ID}\"|g" $config_toml
    # copy the main node's genesis to the peer nodes to ensure they all have the same genesis
    cp $MAIN_GENESIS $genesis_json

    rm -rf ${config_toml}-E
done

# Cleanup from seds
rm -rf ${MAIN_CONFIG}-E
rm -rf ${MAIN_GENESIS}-E