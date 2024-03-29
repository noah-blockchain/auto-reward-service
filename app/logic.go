package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/noah-blockchain/Auto-rewards/config"
	"github.com/noah-blockchain/Auto-rewards/models"
	"github.com/noah-blockchain/Auto-rewards/utils"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"
)

type AutoRewards struct {
	cfg config.Config
}

func NewAutoRewards(cfg config.Config) AutoRewards {
	return AutoRewards{cfg: cfg}
}

func (a AutoRewards) GetValidators() (*models.ValidatorList, error) {

	contentsStatus, err := createReq(fmt.Sprintf("%s/status", a.cfg.NodeApiURL))
	if err != nil {
		return nil, err
	}

	nodeStatus := models.NodeStatus{}
	if err = json.Unmarshal(contentsStatus, &nodeStatus); err != nil {
		return nil, err
	}
	log.Println("INFO! Latest Block Height=", nodeStatus.Result.LatestBlockHeight)

	contentsValidators, err := createReq(fmt.Sprintf("%s/validators?height=%s", a.cfg.NodeApiURL, nodeStatus.Result.LatestBlockHeight))
	if err != nil {
		return nil, err
	}

	validatorList := models.ValidatorList{}
	if err = json.Unmarshal(contentsValidators, &validatorList); err != nil {
		return nil, err
	}
	log.Println("INFO! Validators count=", len(validatorList.Validators))

	return &validatorList, nil
}

func (a AutoRewards) GetDelegatorsListByNode(pubKey string) (map[string]float64, error) {
	contents, err := createReq(fmt.Sprintf("%s/api/v1/validators/%s", a.cfg.ExplorerApiURL, pubKey))
	if err != nil {
		return nil, err
	}

	validatorInfo := models.ValidatorInfo{}
	if err = json.Unmarshal(contents, &validatorInfo); err != nil {
		return nil, err
	}
	log.Println("INFO! Delegator count =", validatorInfo.Result.DelegatorCount)

	if validatorInfo.Result.DelegatorCount == 0 {
		return map[string]float64{}, nil
	}

	values := make(map[string]float64, validatorInfo.Result.DelegatorCount)
	for _, delegator := range validatorInfo.Result.DelegatorList {
		if delegator.Coin == a.cfg.Token {
			if value, err := strconv.ParseFloat(delegator.Value, 64); err == nil {
				values[delegator.Address] = value
				log.Println("INFO! Address=", delegator.Value, "Value=", value)
			}
		}
	}

	return values, nil
}

func (a AutoRewards) GetAllDelegators() (map[string]float64, error) {
	allValidators, err := a.GetValidators()
	if err != nil {
		return nil, err
	}

	allDelegators := make(map[string]float64)
	for _, validator := range allValidators.Validators {
		delegators, err := a.GetDelegatorsListByNode(validator.PubKey)
		if err != nil {
			continue
		}

		if len(delegators) == 0 {
			continue
		}

		if len(allDelegators) > 0 {
			for address, amount := range delegators {
				if val, ok := allDelegators[address]; ok {
					allDelegators[address] += val
				} else {
					allDelegators[address] = amount
				}
			}
		} else {
			allDelegators = delegators
		}
	}

	return allDelegators, nil
}

func (a AutoRewards) getAllPayedDelegators() (map[string]float64, error) {
	allDelegators, err := a.GetAllDelegators()
	if err != nil {
		return nil, err
	}

	allPayedDelegators := make(map[string]float64)
	for address, _ := range allDelegators {
		amounts := allDelegators[address]
		allPayedDelegators[address] = amounts
	}
	return allPayedDelegators, nil
}

func (a AutoRewards) getTotalDelegatedCoins() (float64, map[string]float64, error) {
	payedDelegatorsList, err := a.getAllPayedDelegators()
	if err != nil {
		return 0.0, nil, err
	}

	totalDelegatedCoins := 0.0
	for _, amount := range payedDelegatorsList {
		totalDelegatedCoins += amount
	}

	return totalDelegatedCoins, payedDelegatorsList, nil
}

func (a AutoRewards) getWalletBalances(address string) (*models.AddressInfo, error) {
	contents, err := createReq(fmt.Sprintf("%s/api/v1/addresses/%s", os.Getenv("EXPLORER_API_URL"), address))
	if err != nil {
		return nil, err
	}

	addressInfo := models.AddressInfo{}
	if err = json.Unmarshal(contents, &addressInfo); err != nil {
		return nil, err
	}
	log.Println("INFO! Address=", addressInfo.Data.Address, "Balance=", addressInfo.Data.Balances)

	return &addressInfo, nil
}

func (a AutoRewards) getNoahBalance(address string) (float64, error) {
	balances, err := a.getWalletBalances(address)
	if err != nil {
		return 0.0, err
	}

	for _, value := range balances.Data.Balances {
		if value.Coin == a.cfg.BaseCoin {
			if value, err := strconv.ParseFloat(value.Amount, 64); err == nil {
				return value, nil
			}
		}
	}

	return 0.0, errors.New("ERROR! User Noah Balance not found")
}

func (a AutoRewards) CreateMultiSendList(walletFrom string, payCoinName string) ([]models.MultiSendItem, error) {
	totalDelegatedCoins, payedDelegatedList, err := a.getTotalDelegatedCoins()
	if err != nil || totalDelegatedCoins == 0 || len(payedDelegatedList) == 0{
		return nil, err
	}

	//payedDelegatedList, err := a.getAllPayedDelegators()
	//if err != nil {
	//	return nil, err
	//}
	log.Println("INFO! Payed Delegated List Count =", len(payedDelegatedList))

	var balanceNoah float64
	for {
		if balanceNoah, err = a.getNoahBalance(walletFrom); err == nil && balanceNoah > 0.0 {
			log.Println(fmt.Sprintf("OK! Received Noah Balance %f", balanceNoah))
			break
		}
		log.Println(err)
		time.Sleep(5 * time.Second)
	}

	sumQNoah := big.NewInt(0)
	toBePayed := balanceNoah * 0.95
	var multiSendList []models.MultiSendItem
	for address, amount := range payedDelegatedList {
		percent := amount * 100 / totalDelegatedCoins
		valueQNoah := utils.FloatToBigInt(toBePayed * percent * 0.01)
		log.Println(address, "receiving", percent, "%", valueQNoah, "QNOAH")
		multiSendList = append(multiSendList, models.MultiSendItem{
			Coin:  payCoinName,
			To:    address,
			Value: valueQNoah,
		})
		sumQNoah.Add(sumQNoah, valueQNoah)
	}
	log.Println("INFO! Will be send", sumQNoah.String(), "QNOAH")

	if sumQNoah.String() == "0" {
		return nil, errors.New("ERROR! Trying send 0 QNOAH")
	}

	return multiSendList, nil
}
