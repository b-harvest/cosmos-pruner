package cmd

import (
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"encoding/binary"
	"fmt"
	"github.com/bharvest-devops/cosmos-pruner/internal/rootmulti"
	"github.com/cockroachdb/pebble"
	cometdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/state"
	"github.com/cometbft/cometbft/store"
	dbm "github.com/cosmos/cosmos-db"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// to figuring out the height to prune tx_index
var txIdxHeight int64 = 0

// load dbm
// load app store and prune
// if immutable tree is not deletable we should import and export current state

func pruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune [path_to_home]",
		Short: "prune data from the application store and block store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			//ctx := cmd.Context()
			//errs, _ := errgroup.WithContext(ctx)
			var err error
			if tendermint {
				if err = pruneTMData(args[0]); err != nil {
					fmt.Println(err.Error())
				}
			}

			if cosmosSdk {
				err = pruneAppState(args[0])
				if err != nil {
					fmt.Println(err.Error())
				}
			}

			if tx_idx {
				err = pruneTxIndex(args[0])
				if err != nil {
					fmt.Println(err.Error())
				}
			}

			return nil
		},
	}
	return cmd
}

func pruneTxIndex(home string) error {
	fmt.Println("pruning tx_index")
	txIdxDB, err := openCosmosDB("tx_index", home)
	if err != nil {
		return err
	}

	defer func() {
		errClose := txIdxDB.Close()
		if errClose != nil {
			fmt.Println(errClose.Error())
		}
	}()

	pruneHeight := txIdxHeight - int64(blocks) - 10
	if pruneHeight <= 0 {
		fmt.Printf("No need to prune (pruneHeight=%d)\n", pruneHeight)
		return nil
	}

	pruneBlockIndex(txIdxDB, pruneHeight)
	pruneTxIndexTxs(txIdxDB, pruneHeight)

	fmt.Println("finished pruning tx_index")

	if compact {
		fmt.Println("compacting tx_index")
		if err := compactCosmosDB(txIdxDB); err != nil {
			fmt.Println(err.Error())
		}
	}

	return nil
}

func pruneTxIndexTxs(db dbm.DB, pruneHeight int64) {
	itr, itrErr := db.Iterator(nil, nil)
	if itrErr != nil {
		panic(itrErr)
	}

	defer itr.Close()

	///////////////////////////////////////////////////
	// delete index by hash and index by height
	for ; itr.Valid(); itr.Next() {
		key := itr.Key()
		value := itr.Value()

		strKey := string(key)

		if strings.HasPrefix(strKey, "tx.height") { // index by height
			strs := strings.Split(strKey, "/")
			intHeight, _ := strconv.ParseInt(strs[2], 10, 64)

			if intHeight < pruneHeight {
				db.Delete(value)
				db.Delete(key)
			}
		} else {
			if len(value) == 32 { // maybe index tx by events
				strs := strings.Split(strKey, "/")
				if len(strs) == 4 { // index tx by events
					intHeight, _ := strconv.ParseInt(strs[2], 10, 64)
					if intHeight < pruneHeight {
						db.Delete(key)
					}
				}
			}
		}
	}
}

func pruneBlockIndex(db dbm.DB, pruneHeight int64) {
	itr, itrErr := db.Iterator(nil, nil)
	if itrErr != nil {
		panic(itrErr)
	}

	defer itr.Close()

	for ; itr.Valid(); itr.Next() {
		key := itr.Key()
		value := itr.Value()

		strKey := string(key)

		if strings.HasPrefix(strKey, "block.height") /* index block primary key*/ || strings.HasPrefix(strKey, "block_events") /* BeginBlock & EndBlock */ {
			intHeight := int64FromBytes(value)
			//fmt.Printf("intHeight: %d\n", intHeight)

			if intHeight < pruneHeight {
				db.Delete(key)
			}
		}
	}
}

func pruneAppState(home string) error {
	appDB, errDB := openCosmosDB("application", home)
	if errDB != nil {
		return errDB
	}

	defer appDB.Close()

	var err error

	//TODO: need to get all versions in the store, setting randomly is too slow
	fmt.Println("pruning application state")

	//// only mount keys from core sdk
	//// todo allow for other keys to be mounted
	//keys := types.NewKVStoreKeys(
	//	authtypes.StoreKey, banktypes.StoreKey, stakingtypes.StoreKey,
	//	minttypes.StoreKey, distrtypes.StoreKey, slashingtypes.StoreKey,
	//	govtypes.StoreKey, paramstypes.StoreKey, ibchost.StoreKey, upgradetypes.StoreKey,
	//	evidencetypes.StoreKey, ibctransfertypes.StoreKey, capabilitytypes.StoreKey,
	//)

	keys := getStoreKeys(appDB)

	// TODO: cleanup app state
	appStore := rootmulti.NewStore(appDB, log.NewNopLogger())

	if txIdxHeight <= 0 {
		txIdxHeight = appStore.LastCommitID().Version
		fmt.Printf("[pruneAppState] set txIdxHeight=%d\n", txIdxHeight)
	}

	for _, value := range keys {
		appStore.MountStoreWithDB(storetypes.NewKVStoreKey(value), storetypes.StoreTypeIAVL, nil)
	}

	err = appStore.LoadLatestVersion()
	if err != nil {
		return err
	}

	allVersions := appStore.GetAllVersions()

	v64 := make([]int64, len(allVersions))
	for i := 0; i < len(allVersions); i++ {
		v64[i] = int64(allVersions[i])
	}

	versionsToPrune := int64(len(v64)) - int64(versions)
	fmt.Printf("[pruneAppState] versionsToPrune=%d\n", versionsToPrune)
	if versionsToPrune <= 0 {
		fmt.Printf("[pruneAppState] No need to prune (%d)\n", versionsToPrune)
	} else {
		var (
			pruningHeight int64
			i             int
		)
		for {
			pruningHeight = appStore.GetPruningHeight(versionsToPrune)
			if i > 10000 {
				panic("Could not found pruning height!! you should check if your storage is healthy")
			}
			if pruningHeight == 0 {
				versionsToPrune--
				i++
				continue
			}
			break
		}

		err = appStore.PruneStores(pruningHeight)
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	if compact {
		fmt.Println("compacting application state")
		if err := compactCosmosDB(appDB); err != nil {
			fmt.Println(err.Error())
		}
	}

	return nil
}

// pruneTMData prunes the tendermint blocks and state based on the amount of blocks to keep
func pruneTMData(home string) error {
	blockStoreDB, errDBBlock := openCometBFTDB("blockstore", home)
	if errDBBlock != nil {
		return errDBBlock
	}

	blockStore := store.NewBlockStore(blockStoreDB)
	defer blockStore.Close()

	// Get StateStore
	stateDB, errDBBState := openCometBFTDB("state", home)
	if errDBBState != nil {
		return errDBBState
	}

	var err error

	stateStore := state.NewStore(stateDB, state.StoreOptions{})
	defer stateStore.Close()

	base := blockStore.Base()

	pruneHeight := blockStore.Height() - int64(blocks)
	fmt.Printf("[pruneTMData] pruneHeight=%d\n", pruneHeight)
	if pruneHeight <= 0 {
		fmt.Println("[pruneTMData] No need to prune")
		return nil
	}

	if txIdxHeight <= 0 {
		txIdxHeight = blockStore.Height()
		fmt.Printf("[pruneTMData] set txIdxHeight=%d\n", txIdxHeight)
	}

	fmt.Println("pruning block/state store")
	state, err := stateStore.Load()

	var (
		prunedBlocksCount uint64
		endHeight         int64 = base
	)

	// prune block store
	// prune one by one instead of range to avoid `panic: pebble: batch too large: >= 4.0 G` issue
	// (see https://github.com/notional-labs/cosmprund/issues/11)
	for pruneStateFrom := base; pruneStateFrom < pruneHeight-1; pruneStateFrom += rootmulti.PRUNE_BATCH_SIZE {
		err = nil
		height := pruneStateFrom
		if height >= pruneHeight-1 {
			height = pruneHeight - 1
		}

		prunedBlocks, evidenceRetainBlocks, _ := blockStore.PruneBlocks(height, state)
		if err != nil {
			//return err
			fmt.Println(err.Error())
		}
		prunedBlocksCount += prunedBlocks

		endHeight += rootmulti.PRUNE_BATCH_SIZE
		if endHeight >= pruneHeight-1 {
			endHeight = pruneHeight - 1
		}

		_, err = stateStore.LoadConsensusParams(endHeight)
		if err != nil {
			continue
		}
		_, err = stateStore.LoadValidators(endHeight)
		if err != nil {
			continue
		}
		_, err = stateStore.LoadFinalizeBlockResponse(endHeight)
		if err != nil {
			continue
		}
		_, err = stateStore.LoadLastFinalizeBlockResponse(endHeight)
		if err != nil {
			continue
		}

		err = stateStore.PruneStates(pruneStateFrom, endHeight, evidenceRetainBlocks)
		if err != nil {
			fmt.Printf("failed to prune state store: %s", err)
		}
	}

	fmt.Printf("Pruned blocks count: %d\n", prunedBlocksCount)

	if compact {
		fmt.Println("compacting block store")
		if err := compactCometBFTDB(blockStoreDB); err != nil {
			fmt.Println(err.Error())
		}
	}

	if compact {
		fmt.Println("compacting state store")
		if err := compactCometBFTDB(stateDB); err != nil {
			fmt.Println(err.Error())
		}
	}

	return nil
}

// Utils
func openCosmosDB(dbname string, home string) (dbm.DB, error) {
	dbType := dbm.BackendType(backend)
	dbDir := rootify(dataDir, home)

	var db1 dbm.DB

	if dbType == dbm.GoLevelDBBackend {
		o := opt.Options{
			DisableSeeksCompaction: true,
		}

		lvlDB, err := dbm.NewGoLevelDBWithOpts(dbname, dbDir, &o)
		if err != nil {
			return nil, err
		}

		db1 = lvlDB
	} else if dbType == dbm.PebbleDBBackend {
		ppDB, err := dbm.NewPebbleDB(dbname, dbDir, dbm.OptionsMap{})
		if err != nil {
			return nil, err
		}

		db1 = ppDB
	} else {
		var err error
		db1, err = dbm.NewDB(dbname, dbType, dbDir)
		if err != nil {
			return nil, err
		}
	}

	return db1, nil
}

// Utils
func openCometBFTDB(dbname string, home string) (cometdb.DB, error) {
	dbType := cometdb.BackendType(backend)
	dbDir := rootify(dataDir, home)

	var db1 cometdb.DB

	if dbType == cometdb.GoLevelDBBackend {
		o := opt.Options{
			DisableSeeksCompaction: true,
		}

		lvlDB, err := cometdb.NewGoLevelDBWithOpts(dbname, dbDir, &o)
		if err != nil {
			return nil, err
		}

		db1 = lvlDB
	} else if dbType == cometdb.PebbleDBBackend {
		opts := &pebble.Options{
			//DisableAutomaticCompactions: true, // freeze when pruning!
		}
		opts.EnsureDefaults()

		ppDB, err := cometdb.NewPebbleDBWithOpts(dbname, dbDir, opts)
		if err != nil {
			return nil, err
		}

		db1 = ppDB
	} else {
		var err error
		db1, err = cometdb.NewDB(dbname, dbType, dbDir)
		if err != nil {
			return nil, err
		}
	}

	return db1, nil
}

func compactCosmosDB(vdb dbm.DB) error {
	dbType := dbm.BackendType(backend)

	if dbType == dbm.GoLevelDBBackend {
		vdbLevel := vdb.(*dbm.GoLevelDB)

		if err := vdbLevel.ForceCompact(nil, nil); err != nil {
			return err
		}
	} else if dbType == dbm.PebbleDBBackend {
		vdbPebble := vdb.(*dbm.PebbleDB).DB()

		iter, _ := vdbPebble.NewIter(nil)
		//defer iter.Close()

		var start, end []byte

		if iter.First() {
			start = cp(iter.Key())
		}

		if iter.Last() {
			end = cp(iter.Key())
		}

		// close iter before compacting
		iter.Close()

		err := vdbPebble.Compact(start, end, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func compactCometBFTDB(vdb cometdb.DB) error {
	dbType := cometdb.BackendType(backend)

	if dbType == cometdb.GoLevelDBBackend {
		vdbLevel := vdb.(*cometdb.GoLevelDB)

		if err := vdbLevel.Compact(nil, nil); err != nil {
			return err
		}
	} else if dbType == cometdb.PebbleDBBackend {
		vdbPebble := vdb.(*cometdb.PebbleDB).DB()

		iter, _ := vdbPebble.NewIter(nil)
		//defer iter.Close()

		var start, end []byte

		if iter.First() {
			start = cp(iter.Key())
		}

		if iter.Last() {
			end = cp(iter.Key())
		}

		// close iter before compacting
		iter.Close()

		err := vdbPebble.Compact(start, end, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func getStoreKeys(db dbm.DB) (storeKeys []string) {
	latestVer := rootmulti.GetLatestVersion(db)
	latestCommitInfo, err := getCommitInfo(db, latestVer)
	if err != nil {
		panic(err)
	}

	for _, storeInfo := range latestCommitInfo.StoreInfos {
		storeKeys = append(storeKeys, storeInfo.Name)
	}
	return
}

func getCommitInfo(db dbm.DB, ver int64) (*storetypes.CommitInfo, error) {
	const commitInfoKeyFmt = "s/%d" // s/<version>
	cInfoKey := fmt.Sprintf(commitInfoKeyFmt, ver)

	bz, err := db.Get([]byte(cInfoKey))
	if err != nil {
		return nil, fmt.Errorf("failed to get commit info: %s", err)
	} else if bz == nil {
		return nil, fmt.Errorf("no commit info found")
	}

	cInfo := &storetypes.CommitInfo{}
	if err = cInfo.Unmarshal(bz); err != nil {
		return nil, fmt.Errorf("failed unmarshal commit info: %s", err)
	}

	return cInfo, nil
}

func cp(bz []byte) (ret []byte) {
	ret = make([]byte, len(bz))
	copy(ret, bz)
	return ret
}

func rootify(path, root string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func int64FromBytes(bz []byte) int64 {
	v, _ := binary.Varint(bz)
	return v
}
