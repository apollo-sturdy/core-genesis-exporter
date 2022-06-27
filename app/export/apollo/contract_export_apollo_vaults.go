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
	apolloFactory      = "terra1g7jjjkt5uvkjeyhp8ecdz4e4hvtn83sud3tmh2"
	cfeVesting         = "terra1878h54yx347vxnlx8e0la9ngdnqu4uw9u2ppma"
	astroportGenerator = "terra1zgrx9jjqrfye8swykfgmd6hpde60j0nszzupp9"
	astroportLockdrop  = "terra1627ldjvxatt54ydd3ns6xaxtd68a2vtyu7kakj"
	apolloUstAstroLp   = "terra1zuktmswe9zjck0xdpw2k79t0crjk86fljv2rm0"
	apolloToken        = "terra100yeqvww74h4yaejj6h733thgcafdaukjtw397"
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
	Staker        string    `json:"staker"`
	BondAmount    types.Int `json:"bond_amount"`
	PendingReward types.Int `json:"pending_reward"`
}

type GetTotalCfeRewardsResponse struct {
	PendingReward          types.Int `json:"pending_reward"`
	ExtensionPendingReward types.Int `json:"extension_pending_reward"`
}

type CfeAccountInfoResponse struct {
	Phase1Claimable               types.Int `json:"phase1_claimable_amount"`
	Phase2Claimable               types.Int `json:"phase2_claimable_amount"`
	PendingRewardCLaimed          types.Int `json:"pending_reward_claimed"`
	ExtensionPendingRewardClaimed types.Int `json:"extension_pending_reward_claimed"`
	LastClaimedPhase1             int       `json:"last_claimed_phase1"`
	LastClaimedPhase2             int       `json:"last_claimed_phase2"`
}

type CfeAccountResponse struct {
	Address string                 `json:"address"`
	Info    CfeAccountInfoResponse `json:"info"`
}

type (
	asset struct {
		AssetInfo assetInfo `json:"info"`
		Amount    sdk.Int   `json:"amount"`
	}

	pool struct {
		Assets     [2]asset `json:"assets"`
		TotalShare sdk.Int  `json:"total_share"`
	}

	assetInfo struct {
		Token *struct {
			ContractAddr string `json:"contract_addr"`
		} `json:"token,omitempty"`
		NativeToken *struct {
			Denom string `json:"denom"`
		} `json:"native_token,omitempty"`
	}
)

type AddressWithBalance struct {
	Address    string `json:"address"`
	Balance    string `json:"balance"`
	IsContract bool   `json:"isContract"`
}

func pickDenomOrContractAddress(asset assetInfo) string {
	if asset.Token != nil {
		return asset.Token.ContractAddr
	}

	if asset.NativeToken != nil {
		return asset.NativeToken.Denom
	}

	panic("unknown denom")
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

func ExportVaultRewards(app *terra.TerraApp) ([]AddressWithBalance, error) {
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

	i := 1
	var results []AddressWithBalance
	for address, reward := range pendingRewards {
		_, err := keeper.GetContractInfo(sdk.UnwrapSDKContext(ctx), util.ToAddress(address))
		isContract := err == nil

		results = append(results, AddressWithBalance{
			Address:    address,
			Balance:    reward.String(),
			IsContract: isContract,
		})

		app.Logger().Info(fmt.Sprintf("Got contract info for  %d / %d", i, len(pendingRewards)))
		i++
	}

	app.Logger().Info(fmt.Sprintf("Finished getting vault rewards. Total: %s", total.String()))

	return results, nil
}

func ExportCfeRewards(app *terra.TerraApp) (map[string]CfeAccountInfoResponse, error) {
	app.Logger().Info("Exporting Apollo CFE Rewards 5")
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
	cfeAccounts := make(map[string]CfeAccountInfoResponse)
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

		cfeAccounts[walletAddr] = rewardsResponse.Info

		app.Logger().Info(fmt.Sprintf("Fetched %d / %d", i, len(addresses)))
		i++
	}

	app.Logger().Info(fmt.Sprintf("Finished getting cfe rewards. Total: %s", total.String()))

	return cfeAccounts, nil
}

func ExportAstroGeneratorHoldings(app *terra.TerraApp) ([]AddressWithBalance, error) {
	app.Logger().Info("Exporting Astroport Generator Holdings")
	ctx := util.PrepCtx(app)
	qs := util.PrepWasmQueryServer(app)
	keeper := app.WasmKeeper

	pairAddr := "terra1zpnhtf9h5s7ze2ewlqyer83sr4043qcq64zfc4"

	var pool pool
	//Query pool
	if err := util.ContractQuery(ctx, qs, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: pairAddr,
		QueryMsg:        []byte("{\"pool\":{}}"),
	}, &pool); err != nil {
		panic(fmt.Errorf("unable to... %v", err))
	}

	//Get Apollo tokens in pool and total LP tokens
	asset0 := pickDenomOrContractAddress(pool.Assets[0].AssetInfo)
	asset1 := pickDenomOrContractAddress(pool.Assets[1].AssetInfo)
	var tokensInPair sdk.Int
	if asset0 == apolloToken {
		tokensInPair = pool.Assets[0].Amount
	} else if asset1 == apolloToken {
		tokensInPair = pool.Assets[1].Amount
	} else {
		panic("Apollo token not found in pair")
	}
	totalShares := pool.TotalShare

	contractAddr, err := sdk.AccAddressFromBech32(astroportGenerator)
	if err != nil {
		return nil, nil
	}

	//Get all lp tokens from generator and convert to apollo
	prefix := util.GeneratePrefix("user_info", apolloUstAstroLp)
	total := sdk.ZeroInt()
	i := 1

	var results []AddressWithBalance

	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), contractAddr, prefix, func(key, value []byte) bool {
		walletAddr := string(key)
		// app.Logger().Info(fmt.Sprintf("Got addr: %s", walletAddr))

		var userInfo struct {
			Amount sdk.Int `json:"amount"`
		}
		util.MustUnmarshalTMJSON(value, &userInfo)

		shareRatio := sdk.NewDecFromInt(userInfo.Amount).Quo(sdk.NewDecFromInt(totalShares))
		usersApolloTokens := shareRatio.MulInt(tokensInPair).TruncateInt()

		_, err := keeper.GetContractInfo(sdk.UnwrapSDKContext(ctx), util.ToAddress(walletAddr))
		isContract := err == nil

		results = append(results, AddressWithBalance{
			Address:    walletAddr,
			Balance:    usersApolloTokens.String(),
			IsContract: isContract,
		})

		total = total.Add(usersApolloTokens)
		app.Logger().Info(fmt.Sprintf("Fetched %d / ?. Total Apollo tokens: %s", i, total.String()))
		i++

		return false
	})

	return results, nil
}

func ExportAstroLockdropHoldings(app *terra.TerraApp) ([]AddressWithBalance, error) {
	app.Logger().Info("Exporting Astroport Lockdrop Holdings, fixed")
	ctx := util.PrepCtx(app)
	qs := util.PrepWasmQueryServer(app)
	keeper := app.WasmKeeper

	astroPairAddr := "terra1zpnhtf9h5s7ze2ewlqyer83sr4043qcq64zfc4"
	terraswapLpToken := "terra1n3gt4k3vth0uppk0urche6m3geu9eqcyujt88q"

	var pool pool
	//Query pool
	if err := util.ContractQuery(ctx, qs, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: astroPairAddr,
		QueryMsg:        []byte("{\"pool\":{}}"),
	}, &pool); err != nil {
		panic(fmt.Errorf("unable to... %v", err))
	}

	//Get Apollo tokens in pool and total LP tokens
	asset0 := pickDenomOrContractAddress(pool.Assets[0].AssetInfo)
	asset1 := pickDenomOrContractAddress(pool.Assets[1].AssetInfo)
	var tokensInPair sdk.Int
	if asset0 == apolloToken {
		tokensInPair = pool.Assets[0].Amount
	} else if asset1 == apolloToken {
		tokensInPair = pool.Assets[1].Amount
	} else {
		panic("Apollo token not found in pair")
	}
	totalShares := pool.TotalShare

	//Get lockdrop deposits into generator
	type poolInfo struct {
		TerraswapAmountInLockup sdk.Int `json:"terraswap_amount_in_lockups"`
		MigrationInfo           struct {
			AstroportLPToken string `json:"astroport_lp_token"`
		} `json:"migration_info"`
	}
	// res, err := qs.RawStore(ctx, &wasmtypes.QueryRawStoreRequest{
	// 	ContractAddress: astroportLockdrop,
	// 	Key:             util.GeneratePrefix(fmt.Sprintf("LiquidityPools%s", terraswapLpToken)),
	// })
	// if err != nil {
	// 	panic(fmt.Errorf("unable to... %v", err))
	// }
	// var pi poolInfo
	// util.MustUnmarshalTMJSON(res.Data, &pi)

	var pi poolInfo
	poolsPrefix := util.GeneratePrefix("LiquidityPools")
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), util.ToAddress(astroportLockdrop), poolsPrefix, func(key, value []byte) bool {
		var lpAddr = string(key)
		if lpAddr == terraswapLpToken {
			util.MustUnmarshalTMJSON(value, &pi)
			return false
		} else {
			return false
		}
	})

	var lpLockedInGenerator sdk.Int
	util.ContractQuery(ctx, qs, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: astroportGenerator,
		QueryMsg:        []byte(fmt.Sprintf("{ \"deposit\": { \"lp_token\": \"%s\", \"user\": \"%s\" } }", pi.MigrationInfo.AstroportLPToken, astroportLockdrop)),
	}, &lpLockedInGenerator)

	//Get all lp tokens from lockdrop and convert to Apollo
	prefix := util.GeneratePrefix("lockup_position", terraswapLpToken)
	var lockupInfo struct {
		LPUnitsLocked          sdk.Int `json:"lp_units_locked"`
		AstroportLPTransferred sdk.Int `json:"astroport_lp_transferred"`
	}

	var results []AddressWithBalance
	total := sdk.ZeroInt()
	i := 1

	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), util.ToAddress(astroportLockdrop), prefix, func(key, value []byte) bool {
		userAddress := string(key[2:46])

		util.MustUnmarshalTMJSON(value, &lockupInfo)

		// If LP transferred is not nil, means the user has withdrawn all LPs after unlock
		if !lockupInfo.LPUnitsLocked.IsNil() && lockupInfo.AstroportLPTransferred.IsNil() {
			terraswapLpLocked := lockupInfo.LPUnitsLocked
			astroLpLocked := terraswapLpLocked.Mul(lpLockedInGenerator).Quo(pi.TerraswapAmountInLockup)

			shareRatio := sdk.NewDecFromInt(astroLpLocked).Quo(sdk.NewDecFromInt(totalShares))
			usersApolloTokens := shareRatio.MulInt(tokensInPair).TruncateInt()

			_, err := keeper.GetContractInfo(sdk.UnwrapSDKContext(ctx), util.ToAddress(userAddress))
			isContract := err == nil

			results = append(results, AddressWithBalance{
				Address:    userAddress,
				Balance:    usersApolloTokens.String(),
				IsContract: isContract,
			})
			total = total.Add(usersApolloTokens)
		}

		app.Logger().Info(fmt.Sprintf("Fetched %d / ?. Total Apollo tokens: %s", i, total.String()))
		i++

		return false
	})

	return results, nil
}

type SpecRewardInfo struct {
	RewardInfo []struct {
		TokenAddr string    `json:"asset_token"`
		LpAmount  types.Int `json:"bond_amount"`
	} `json:"reward_infos"`
}

func getRewardsInfo(ctx context.Context, q wasmtypes.QueryServer, farmAddr string, walletAddr string) (SpecRewardInfo, error) {
	var reward SpecRewardInfo
	err := util.ContractQuery(ctx, q, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: farmAddr,
		QueryMsg:        []byte(fmt.Sprintf("{\"reward_info\":{\"staker_addr\":\"%s\"}}", walletAddr)),
	}, &reward)
	if err != nil {
		return reward, err
	}

	return reward, err
}

func ExportSpecVaultHoldings(app *terra.TerraApp) ([]AddressWithBalance, error) {
	app.Logger().Info("Exporting Spec Vault Holdings")
	ctx := util.PrepCtx(app)
	qs := util.PrepWasmQueryServer(app)
	keeper := app.WasmKeeper
	q := util.PrepWasmQueryServer(app)

	pairAddr := "terra1zpnhtf9h5s7ze2ewlqyer83sr4043qcq64zfc4"

	var pool pool
	//Query pool
	if err := util.ContractQuery(ctx, qs, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: pairAddr,
		QueryMsg:        []byte("{\"pool\":{}}"),
	}, &pool); err != nil {
		panic(fmt.Errorf("unable to... %v", err))
	}

	//Get Apollo tokens in pool and total LP tokens
	asset0 := pickDenomOrContractAddress(pool.Assets[0].AssetInfo)
	asset1 := pickDenomOrContractAddress(pool.Assets[1].AssetInfo)
	var tokensInPair sdk.Int
	if asset0 == apolloToken {
		tokensInPair = pool.Assets[0].Amount
	} else if asset1 == apolloToken {
		tokensInPair = pool.Assets[1].Amount
	} else {
		panic("Apollo token not found in pair")
	}
	totalShares := pool.TotalShare

	var results []AddressWithBalance
	total := sdk.ZeroInt()
	i := 1

	specVault := "terra1zngkjhqqearpfhym9x9hnutpklduz45e9uvp9u"
	farmAddr := util.ToAddress((specVault))

	prefix := util.GeneratePrefix("reward")
	// userLpHoldings := make(map[string]lpHoldings)
	walletSeen := make(map[string]bool)
	keeper.IterateContractStateWithPrefix(sdk.UnwrapSDKContext(ctx), farmAddr, prefix, func(key, value []byte) bool {
		walletAddress := sdk.AccAddress(key[2:22])
		if walletSeen[walletAddress.String()] {
			return false
		}
		walletSeen[walletAddress.String()] = true
		rewards, err := getRewardsInfo(ctx, q, farmAddr.String(), walletAddress.String())
		if err != nil {
			panic(err)
		}

		for _, reward := range rewards.RewardInfo {
			if reward.TokenAddr == apolloToken {
				shareRatio := sdk.NewDecFromInt(reward.LpAmount).Quo(sdk.NewDecFromInt(totalShares))
				usersApolloTokens := shareRatio.MulInt(tokensInPair).TruncateInt()

				_, err := keeper.GetContractInfo(sdk.UnwrapSDKContext(ctx), walletAddress)
				isContract := err == nil

				results = append(results, AddressWithBalance{
					Address:    walletAddress.String(),
					Balance:    usersApolloTokens.String(),
					IsContract: isContract,
				})
				total = total.Add(usersApolloTokens)
			}

		}

		app.Logger().Info(fmt.Sprintf("Fetched %d / ?. Total Apollo tokens: %s", i, total.String()))
		i++

		return false
	})

	return results, nil
}

func ExportApolloAstroVaultHoldings(app *terra.TerraApp) ([]AddressWithBalance, error) {
	app.Logger().Info("Exporting Apollo-UST Astroport LP Apollo Vault Holdings")
	ctx := util.PrepCtx(app)
	qs := util.PrepWasmQueryServer(app)
	keeper := app.WasmKeeper

	pairAddr := "terra1zpnhtf9h5s7ze2ewlqyer83sr4043qcq64zfc4"

	var pool pool
	//Query pool
	if err := util.ContractQuery(ctx, qs, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: pairAddr,
		QueryMsg:        []byte("{\"pool\":{}}"),
	}, &pool); err != nil {
		panic(fmt.Errorf("unable to... %v", err))
	}

	//Get Apollo tokens in pool and total LP tokens
	asset0 := pickDenomOrContractAddress(pool.Assets[0].AssetInfo)
	asset1 := pickDenomOrContractAddress(pool.Assets[1].AssetInfo)
	var tokensInPair sdk.Int
	if asset0 == apolloToken {
		tokensInPair = pool.Assets[0].Amount
	} else if asset1 == apolloToken {
		tokensInPair = pool.Assets[1].Amount
	} else {
		panic("Apollo token not found in pair")
	}
	totalShares := pool.TotalShare

	var results []AddressWithBalance
	total := sdk.ZeroInt()
	i := 1

	strat := util.ToAddress("terra1x7v7qvumfl36g5jh0mtqx3c4g8c35sn0sqfuqp")

	lpHoldings, _, err := getLpHoldingsForStrat(ctx, app.WasmKeeper, strat)
	if err != nil {
		panic(err)
	}

	for walletAddr, lpBalance := range lpHoldings {
		shareRatio := sdk.NewDecFromInt(lpBalance).Quo(sdk.NewDecFromInt(totalShares))
		usersApolloTokens := shareRatio.MulInt(tokensInPair).TruncateInt()

		_, err := keeper.GetContractInfo(sdk.UnwrapSDKContext(ctx), util.ToAddress(walletAddr))
		isContract := err == nil

		results = append(results, AddressWithBalance{
			Address:    walletAddr,
			Balance:    usersApolloTokens.String(),
			IsContract: isContract,
		})
		total = total.Add(usersApolloTokens)

		app.Logger().Info(fmt.Sprintf("Fetched %d / %d. Total Apollo tokens: %s", i, len(lpHoldings), total.String()))
		i++
	}

	return results, nil
}

func ExportApolloTerraswapVaultHoldings(app *terra.TerraApp) ([]AddressWithBalance, error) {
	app.Logger().Info("Exporting Apollo-UST Terraswap LP Apollo Vault Holdings")
	ctx := util.PrepCtx(app)
	qs := util.PrepWasmQueryServer(app)
	keeper := app.WasmKeeper

	pairAddr := "terra1xj2w7w8mx6m2nueczgsxy2gnmujwejjeu2xf78"

	var pool pool
	//Query pool
	if err := util.ContractQuery(ctx, qs, &wasmtypes.QueryContractStoreRequest{
		ContractAddress: pairAddr,
		QueryMsg:        []byte("{\"pool\":{}}"),
	}, &pool); err != nil {
		panic(fmt.Errorf("unable to... %v", err))
	}

	//Get Apollo tokens in pool and total LP tokens
	asset0 := pickDenomOrContractAddress(pool.Assets[0].AssetInfo)
	asset1 := pickDenomOrContractAddress(pool.Assets[1].AssetInfo)
	var tokensInPair sdk.Int
	if asset0 == apolloToken {
		tokensInPair = pool.Assets[0].Amount
	} else if asset1 == apolloToken {
		tokensInPair = pool.Assets[1].Amount
	} else {
		panic("Apollo token not found in pair")
	}
	totalShares := pool.TotalShare

	var results []AddressWithBalance
	total := sdk.ZeroInt()
	i := 1

	strat := util.ToAddress("terra14ge98vxgp3ey90d38wwk9xu73wydjz8vd66h3f")

	lpHoldings, _, err := getLpHoldingsForStrat(ctx, app.WasmKeeper, strat)
	if err != nil {
		panic(err)
	}

	for walletAddr, lpBalance := range lpHoldings {
		shareRatio := sdk.NewDecFromInt(lpBalance).Quo(sdk.NewDecFromInt(totalShares))
		usersApolloTokens := shareRatio.MulInt(tokensInPair).TruncateInt()

		_, err := keeper.GetContractInfo(sdk.UnwrapSDKContext(ctx), util.ToAddress(walletAddr))
		isContract := err == nil

		results = append(results, AddressWithBalance{
			Address:    walletAddr,
			Balance:    usersApolloTokens.String(),
			IsContract: isContract,
		})
		total = total.Add(usersApolloTokens)

		app.Logger().Info(fmt.Sprintf("Fetched %d / %d. Total Apollo tokens: %s", i, len(lpHoldings), total.String()))
		i++
	}

	return results, nil
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
