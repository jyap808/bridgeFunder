package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	ERC20 "github.com/jyap808/bridgeFunder/contracts"
)

type payload struct {
	Username  string  `json:"username"`
	AvatarURL string  `json:"avatar_url"`
	Embeds    []embed `json:"embeds"`
}

type embed struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

var (
	lastTXID string
)

const (
	// Environment variables
	SenderPrivateKeyEnvKeyName    = "SENDER_PRIVATE_KEY"
	WrappedAddressEnvKeyName      = "WRAPPED_ADDRESS"
	Erc20HandlerAddressEnvKeyName = "ERC20_HANDLER_ADDRESS"
	BaseAssetEnvKeyName           = "BASE_ASSET"
	FundLimitWeiEnvKeyName        = "FUND_LIMIT_WEI"
	FundAmountWeiEnvKeyName       = "FUND_AMOUNT_WEI"
	RpcURLEnvKeyName              = "RPC_URL"
	WsRPCURLEnvKeyName            = "WS_RPC_URL"
	WebhookURLEnvKeyName          = "WEBHOOK_URL"
	AvatarUsernameEnvKeyName      = "AVATAR_USERNAME"
	AvatarURLEnvKeyName           = "AVATAR_URL"
)

func main() {
	// Load environment variables
	rpcURL := os.Getenv(RpcURLEnvKeyName)
	wsRPCURL := os.Getenv(WsRPCURLEnvKeyName)
	erc20HandlerAddress := os.Getenv(Erc20HandlerAddressEnvKeyName)
	baseAsset := os.Getenv(BaseAssetEnvKeyName)
	fundLimitWei, _ := strconv.ParseInt(os.Getenv(FundLimitWeiEnvKeyName), 10, 64)
	fundAmountWei, _ := strconv.ParseInt(os.Getenv(FundAmountWeiEnvKeyName), 10, 64)

	clientWS, err := ethclient.Dial(wsRPCURL)
	if err != nil {
		log.Fatalln(err)
	}
	defer clientWS.Close()

	clientRPC, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatal(err)
	}

	subch := make(chan types.Log)

	go func() {
		for i := 0; ; i++ {
			if i > 0 {
				time.Sleep(2 * time.Second)
			}
			subscribeFilterLogs(clientWS, subch)
		}
	}()

	contractAbi, err := abi.JSON(strings.NewReader(string(ERC20.ERC20ABI)))
	if err != nil {
		log.Fatal(err)
	}

	logTransferEvent := common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	for vLog := range subch {
		if len(vLog.Topics) == 0 {
			continue
		}
		switch vLog.Topics[0].Hex() {
		case logTransferEvent.Hex():
			var transfer ERC20.ERC20Transfer
			err := contractAbi.UnpackIntoInterface(&transfer, "Transfer", vLog.Data)
			if err != nil {
				log.Fatal(err)
			}

			transfer.From = common.HexToAddress(vLog.Topics[1].Hex())
			transfer.To = common.HexToAddress(vLog.Topics[2].Hex())

			msg := ""
			if transfer.From.Hex() != erc20HandlerAddress {
				break
			}

			if vLog.TxHash.String() != lastTXID {
				fundTXID, err := fundAddress(clientRPC, transfer.To, fundLimitWei, fundAmountWei)
				if err != nil {
					log.Println("Error:", err)
				} else {
					msg = fmt.Sprintf("Funded - To: %s Value: %.8f %s", transfer.To.Hex(), weiToEther(big.NewInt(fundAmountWei)), baseAsset)
					log.Println(msg, "TXID: ", fundTXID)
					postDiscord(msg, vLog.BlockNumber, fundTXID)
				}
				lastTXID = vLog.TxHash.String()
			} else {
				log.Println("Duplicate TX: ", lastTXID)
			}
		}
	}
}

func fundAddress(client *ethclient.Client, recipientAddress common.Address, fundLimitWei int64, fundAmountWei int64) (txid string, err error) {
	senderPrivateKey := strings.TrimPrefix(os.Getenv(SenderPrivateKeyEnvKeyName), "0x")

	privateKey, err := crypto.HexToECDSA(senderPrivateKey)
	if err != nil {
		log.Fatal(err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("Public Key Error")
	}

	senderAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	balance := balanceCheck(client, recipientAddress)

	fundLimit := big.NewInt(fundLimitWei)
	if result := balance.Cmp(fundLimit); result > 0 {
		return "", fmt.Errorf("balance above threshold")
	}

	nonce, err := client.PendingNonceAt(context.Background(), senderAddress)
	if err != nil {
		return "", fmt.Errorf("unable to get PendingNonceAt of sender")
	}

	gasLimit := uint64(21000) // in units
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return "", fmt.Errorf("unable to get SuggestGasPrice")
	}

	value := big.NewInt(fundAmountWei)
	tx := types.NewTransaction(nonce, recipientAddress, value, gasLimit, gasPrice, nil)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return "", fmt.Errorf("unable to get NetworkID")
	}

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return "", fmt.Errorf("unable to SignTx")
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return "", fmt.Errorf("unable to SendTransaction")
	}

	return signedTx.Hash().Hex(), nil
}

func weiToEther(wei *big.Int) *big.Float {
	if len(wei.Bits()) == 0 {
		return nil
	}
	return new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(params.Ether))
}

func balanceCheck(client *ethclient.Client, address common.Address) *big.Int {
	balance, err := client.BalanceAt(context.Background(), address, nil)
	if err != nil {
		log.Fatal(err)
	}

	return balance
}

func subscribeFilterLogs(client *ethclient.Client, subch chan types.Log) {
	// Load
	wrappedAddress := os.Getenv(WrappedAddressEnvKeyName)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	contractAddress := common.HexToAddress(wrappedAddress)
	query := ethereum.FilterQuery{
		Addresses: []common.Address{
			contractAddress,
		},
	}

	// Subscribe to log events
	sub, err := client.SubscribeFilterLogs(ctx, query, subch)
	if err != nil {
		log.Println("subscribe error:", err)
		return
	}

	// The connection is established now.
	// Update the channel
	var logs types.Log
	subch <- logs

	// The subscription will deliver events to the channel. Wait for the
	// subscription to end for any reason, then loop around to re-establish
	// the connection.
	log.Println("connection lost: ", <-sub.Err())
}

func postDiscord(msg string, block uint64, txid string) {
	// Load
	webhookURL := os.Getenv(WebhookURLEnvKeyName)
	avatarUsername := os.Getenv(AvatarUsernameEnvKeyName)
	avatarURL := os.Getenv(AvatarURLEnvKeyName)

	title := fmt.Sprintf("Block: %d TX: %.30s...", block, txid)
	titleURL := fmt.Sprintf("https://ubiqscan.io/tx/%s", txid)

	blockEmbed := embed{Title: title, URL: titleURL, Description: msg}
	embeds := []embed{blockEmbed}
	jsonReq := payload{Username: avatarUsername, AvatarURL: avatarURL, Embeds: embeds}

	jsonStr, _ := json.Marshal(jsonReq)
	log.Println("JSON POST:", string(jsonStr))

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonStr))
	if err != nil {
		log.Println(err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
}
