# Cosmos-Pruner using cosmos SDK 50


## Usage

```
# clone & build cosmprund repo
git clone https://github.com/bharvest-devops/cosmos-pruner
cd cosmos-pruner
make build

# run cosmos-pruner 
./build/cosmos-pruner prune ~/.gaiad/data
# it'll prune as everything mode.
```

Flags: 

- `blocks`: amount of blocks to keep on the node (Default 10)
- `versions`: amount of app state versions to keep on the node (Default 10). If this value is set, pruner will set a pruning height that is less than the entered value with leaves 0 when divided by 10 (because cosmos-pruner uses everything strategies). 
- `cosmos-sdk`: If pruning app state (Default true)
- `tendermint`: If pruning tendermint data including blockstore and state. (Default true)
- `tx_index`: set to false you dont want to prune tx_index.dbm (default true)
- `compact`: set to false you dont want to compact dbs after prunning (default true)
