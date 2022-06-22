package apollo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	terra "github.com/terra-money/core/app"
	util "github.com/terra-money/core/app/export/util"
	"github.com/terra-money/core/x/wasm/keeper"
	wasmtypes "github.com/terra-money/core/x/wasm/types"
)

var (
	apolloFactory = "terra1g7jjjkt5uvkjeyhp8ecdz4e4hvtn83sud3tmh2"
	cfeVesting    = "terra1878h54yx347vxnlx8e0la9ngdnqu4uw9u2ppma"
)

type Strategy struct {
	Address string `json:"address"`
}

type StrategyInfo struct {
	TotalBondAmount types.Int `json:"total_bond_amount"`
	TotalShares     types.Int `json:"total_shares"`
}

type StrategyConfig struct {
	LpTokenAddr    string `json:"base_token"`
	StrategyConfig struct {
		AssetToken     string `json:"asset_token"`
		AssetTokenPair string `json:"asset_token_pair"`
	} `json:"strategy_config"`
}

type UserInfo struct {
	Shares types.Int `json:"shares"`
}

type StakerInfoResponse struct {
	Staker        string    `jsons:"staker"`
	BondAmount    types.Int `json:"bond_amount"`
	PendingReward types.Int `json:"pending_reward"`
}

type GetTotalCfeRewardsResponse struct {
	PendingReward          types.Int `json:"pending_reward"`
	ExtensionPendingReward types.Int `json:"extension_pending_reward"`
}

type CfeAccountResponse struct {
	Address string `json:"address"`
	Info    struct {
		Phase1Claimable types.Int `json:"phase1_claimable_amount"`
		Phase2Claimable types.Int `json:"phase2_claimable_amount"`
	} `json:"info"`
}

// Exports all LP ownership from Apollo vaults
// Resulting map is in the following format
// {
//	"farm": {
//   "lp_token_address_1": {
//       "wallet_address": "amount",
//   }
//	}
// }
func ExportApolloVaultLPs(app *terra.TerraApp, snapshot util.SnapshotBalanceAggregateMap) (map[string]map[string]map[string]sdk.Int, error) {
	app.Logger().Info("Exporting Apollo Vaults")
	ctx := util.PrepCtx(app)
	strats, err := getListOfStrategies(ctx, app.WasmKeeper)
	if err != nil {
		log.Println(err)
	}
	// log.Printf("no. of apollo strats: %d\n", len(strats))

	allLpHoldings := make(map[string]map[string]map[string]sdk.Int)
	for _, strat := range strats {
		lpHoldings, lpTokenAddr, err := getLpHoldingsForStrat(ctx, app.WasmKeeper, strat)
		if err != nil {
			panic(err)
		}
		allLpHoldings[strat.String()] = make(map[string]map[string]sdk.Int)
		allLpHoldings[strat.String()][lpTokenAddr.String()] = lpHoldings
	}
	return allLpHoldings, nil
}

func ExportApolloUsers(app *terra.TerraApp) ([]sdk.AccAddress, error) {
	app.Logger().Info("Exporting Apollo Users")
	ctx := util.PrepCtx(app)

	users, err := getListOfUsers(app, ctx, app.WasmKeeper)

	return users, err
}

func ExportVaultRewards(app *terra.TerraApp) (map[string]sdk.Int, error) {
	app.Logger().Info("Exporting Apollo Vault Rewards")
	ctx := util.PrepCtx(app)
	qs := util.PrepWasmQueryServer(app)
	keeper := app.WasmKeeper

	contractAddr, err := sdk.AccAddressFromBech32(apolloFactory)
	if err != nil {
		return nil, nil
	}

	//Get all keys from store
	prefix := util.GeneratePrefix("lm_rewards")
	var keys [][]byte
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), contractAddr, prefix, func(key, value []byte) bool {
		// walletAddr := sdk.AccAddress(key[2:22])
		// strategyId := string(key[22:len(key)])
		keys = append(keys, key)
		return false
	})

	app.Logger().Info(fmt.Sprintf("Got all keys. Len: %d", len(keys)))

	//SmartQuery on all keys
	pendingRewards := make(map[string]sdk.Int)
	total := sdk.ZeroInt()
	for i := 0; i < len(keys); i++ {
		key := keys[i]
		walletAddr := sdk.AccAddress(key[2:22]).String()
		strategyId := string(key[22:])

		var stakerInfoResponse StakerInfoResponse
		if err := util.ContractQuery(ctx, qs, &wasmtypes.QueryContractStoreRequest{
			ContractAddress: apolloFactory,
			QueryMsg:        []byte(fmt.Sprintf("{\"get_staker_info\":{\"staker\":\"%s\",\"strategy_id\":%s}}", walletAddr, strategyId)),
		}, &stakerInfoResponse); err != nil {
			panic(fmt.Errorf("unable to query staker info: %v", err))
		}

		pendingReward := stakerInfoResponse.PendingReward

		if !pendingReward.IsZero() {
			if pendingRewards[walletAddr].IsNil() {
				pendingRewards[walletAddr] = pendingReward
			} else {
				pendingRewards[walletAddr] = pendingRewards[walletAddr].Add(pendingReward)
			}
		}

		total = total.Add(stakerInfoResponse.PendingReward)
		app.Logger().Info(fmt.Sprintf("Fetched %d / %d. Total: %s", i, len(keys), total.String()))
	}

	app.Logger().Info(fmt.Sprintf("Finished getting vault rewards. Total: %s", total.String()))

	return pendingRewards, nil
}

func ExportCfeRewards(app *terra.TerraApp) (map[string]sdk.Int, error) {
	app.Logger().Info("Exporting Apollo CFE Rewards")
	ctx := util.PrepCtx(app)
	qs := util.PrepWasmQueryServer(app)
	keeper := app.WasmKeeper

	contractAddr, err := sdk.AccAddressFromBech32(apolloFactory)
	if err != nil {
		return nil, nil
	}

	//Get all keys from store
	prefix := util.GeneratePrefix("rewards")
	addresses := make(map[string]bool)
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), contractAddr, prefix, func(key, value []byte) bool {
		walletAddr := sdk.AccAddress(key[2:22]).String()
		// strategyId := string(key[22:len(key)])
		_, exists := addresses[walletAddr]
		if !exists {
			addresses[walletAddr] = true
		}
		return false
	})

	app.Logger().Info(fmt.Sprintf("Got all keys. Len: %d", len(addresses)))

	//SmartQuery on all keys
	pendingRewards := make(map[string]sdk.Int)
	total := sdk.ZeroInt()
	i := 1
	for walletAddr := range addresses {
		// strategyId := string(key[22:])

		var rewardsResponse CfeAccountResponse
		if err := util.ContractQuery(ctx, qs, &wasmtypes.QueryContractStoreRequest{
			ContractAddress: cfeVesting,
			QueryMsg:        []byte(fmt.Sprintf("{\"cfe_account\":{\"address\":\"%s\"}}", walletAddr)),
		}, &rewardsResponse); err != nil {
			panic(fmt.Errorf("unable to query staker info: %v", err))
		}

		pendingReward := rewardsResponse.Info.Phase1Claimable.Add(rewardsResponse.Info.Phase2Claimable)

		if !pendingReward.IsZero() {
			if pendingRewards[walletAddr].IsNil() {
				pendingRewards[walletAddr] = pendingReward
			} else {
				pendingRewards[walletAddr] = pendingRewards[walletAddr].Add(pendingReward)
			}
		}

		total = total.Add(pendingReward)
		app.Logger().Info(fmt.Sprintf("Fetched %d / %d. Total: %s", i, len(addresses), total.String()))
		i++
	}

	app.Logger().Info(fmt.Sprintf("Finished getting cfe rewards. Total: %s", total.String()))

	return pendingRewards, nil
}

func ExportStaticVaultLPs(app *terra.TerraApp) (map[string]map[string]map[string]sdk.Int, error) {
	app.Logger().Info("Exporting Apollo Vaults")
	ctx := util.PrepCtx(app)

	astroStaticStrat, err := sdk.AccAddressFromBech32("terra1x7v7qvumfl36g5jh0mtqx3c4g8c35sn0sqfuqp")
	if err != nil {
		log.Println(err)
	}
	terraswapStaticStrat, err := sdk.AccAddressFromBech32("terra14ge98vxgp3ey90d38wwk9xu73wydjz8vd66h3f")
	if err != nil {
		log.Println(err)
	}

	strats := []sdk.AccAddress{astroStaticStrat, terraswapStaticStrat}

	allLpHoldings := make(map[string]map[string]map[string]sdk.Int)
	for _, strat := range strats {
		lpHoldings, lpTokenAddr, err := getLpHoldingsForStrat(ctx, app.WasmKeeper, strat)
		if err != nil {
			panic(err)
		}
		allLpHoldings[strat.String()] = make(map[string]map[string]sdk.Int)
		allLpHoldings[strat.String()][lpTokenAddr.String()] = lpHoldings
	}
	return allLpHoldings, nil
}

func getLpHoldingsForStrat(ctx context.Context, keeper keeper.Keeper, strategyAddr sdk.AccAddress) (map[string]sdk.Int, sdk.AccAddress, error) {
	lpTokenAddr, _, err := getStrategyConfig(ctx, keeper, strategyAddr)
	if err != nil {
		return map[string]sdk.Int{}, lpTokenAddr, err
	}
	// log.Printf("vault: %s, lp token: %s, lp pair: %s\n", strategyAddr, lpTokenAddr, tokenPair)
	stratInfo, err := getStrategyInfo(ctx, keeper, strategyAddr)
	if err != nil {
		return map[string]sdk.Int{}, lpTokenAddr, err
	}
	// log.Printf("%v\n", stratInfo)
	userLpHoldings, err := getUserLpHoldings(ctx, keeper, strategyAddr, stratInfo)
	if err != nil {
		return map[string]sdk.Int{}, lpTokenAddr, err
	}
	// log.Printf("len: %d", len(userLpHoldings))
	return userLpHoldings, lpTokenAddr, nil
}

func getUserLpHoldings(ctx context.Context, keeper keeper.Keeper, strategyAddr sdk.AccAddress, stratInfo StrategyInfo) (map[string]sdk.Int, error) {
	prefix := util.GeneratePrefix("user")
	lpHoldings := make(map[string]sdk.Int)
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), strategyAddr, prefix, func(key, value []byte) bool {
		// fmt.Printf("%x, %s\n", key, value)
		var userInfo UserInfo
		err := json.Unmarshal(value, &userInfo)
		if err != nil {
			panic(err)
		}
		if userInfo.Shares.IsZero() {
			return false
		}
		walletAddr := sdk.AccAddress(key)
		lpAmount := userInfo.Shares.Mul(stratInfo.TotalBondAmount).Quo(stratInfo.TotalShares)
		lpHoldings[walletAddr.String()] = lpAmount
		return false
	})
	return lpHoldings, nil
}

func getStrategyInfo(ctx context.Context, keeper keeper.Keeper, strategyAddr sdk.AccAddress) (StrategyInfo, error) {
	prefix := util.GeneratePrefix("strategy")
	var stratInfo StrategyInfo
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), strategyAddr, prefix, func(key, value []byte) bool {
		// fmt.Printf("%x, %s\n", key, value)
		err := json.Unmarshal(value, &stratInfo)
		if err != nil {
			panic(err)
		}
		return false
	})
	return stratInfo, nil
}

func getStrategyConfig(ctx context.Context, keeper keeper.Keeper, strategyAddr sdk.AccAddress) (sdk.AccAddress, sdk.AccAddress, error) {
	prefix := util.GeneratePrefix("config")
	var stratConfig StrategyConfig
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), strategyAddr, prefix, func(key, value []byte) bool {
		// fmt.Printf("%x, %s\n", key, value)
		err := json.Unmarshal(value, &stratConfig)
		if err != nil {
			panic(err)
		}
		return false
	})
	baseToken, err := util.AccAddressFromBase64(stratConfig.LpTokenAddr)
	if err != nil {
		panic(err)
	}
	tokenPair, err := util.AccAddressFromBase64(stratConfig.StrategyConfig.AssetTokenPair)
	if err != nil {
		panic(err)
	}
	return baseToken, tokenPair, nil
}

func getListOfStrategies(ctx context.Context, keeper keeper.Keeper) ([]sdk.AccAddress, error) {
	contractAddr, err := sdk.AccAddressFromBech32(apolloFactory)
	if err != nil {
		return nil, nil
	}

	prefix := util.GeneratePrefix("strategies")
	var strats []sdk.AccAddress
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), contractAddr, prefix, func(key, value []byte) bool {
		var strat Strategy
		err = json.Unmarshal(value, &strat)
		if err != nil {
			// skip if error parsing json
			return false
		}
		stratAddr, err := util.AccAddressFromBase64(strat.Address)
		if err != nil {
			// skip if error parsing address
			return false
		}
		strats = append(strats, stratAddr)
		return false
	})
	return strats, nil
}

func getListOfUsers(app *terra.TerraApp, ctx context.Context, keeper keeper.Keeper) ([]sdk.AccAddress, error) {
	contractAddr, err := sdk.AccAddressFromBech32(apolloFactory)
	if err != nil {
		return nil, nil
	}

	prefix := util.GeneratePrefix("lm_rewards")
	var users []sdk.AccAddress
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), contractAddr, prefix, func(key, value []byte) bool {
		walletAddr := sdk.AccAddress(key[2:22])
		users = append(users, walletAddr)
		return false
	})
	return users, nil
}
