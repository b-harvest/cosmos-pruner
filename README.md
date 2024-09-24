# Cosmos-Pruner using cosmos SDK 50
Previously, cosmos-pruner was used cosmos-sdk 45.
but after some years, newer version has out and currently we detected fatal error when prune Cosmos storage which has cosmos sdk 50 version.

through this cosmos-pruner, you'll be possible to prune safely for Cosmos SDK 50 version app.


However, Cosmos SDK will continue to evolve and some changes may occur in the future.

So if you're running an unstable chain or want to keep record safely, 
you should avoid using cosmos-pruner prune command and just use only the compact command to reduce goleveldb's disk usage.





## Usage

```
# clone & build cosmprund repo
git clone https://github.com/bharvest-devops/cosmos-pruner
cd cosmos-pruner
make build

# run cosmos-pruner 
./build/cosmprund prune ~/.gaiad/data
# it'll prune as everything mode.
```

Flags: 

- `blocks`: amount of blocks to keep on the node (Default 10)
- `versions`: amount of app state versions to keep on the node (Default 10). If this value is set, pruner will set a pruning height that is less than the entered value and also leaves 0 when divided by 10 (because cosmos-pruner uses everything strategies). 
- `cosmos-sdk`: If pruning app state (Default true)
- `tendermint`: If pruning tendermint data including blockstore and state. (Default true)
- `tx_index`: set to false you dont want to prune tx_index.dbm (default true)
- `compact`: set to false you dont want to compact dbs after prunning (default true)
