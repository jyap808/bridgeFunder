## Bridge Funder

Listens for Bridge In events.

Checks the balance of the address performing a Bridge In.

Fund that account if the balance is under a set limit with the native gas token so they can perform a basic swap. Usual case is bridging in the wrapped native token.

### Set up

Use the [Foundry](https://getfoundry.sh/) tool [Cast](https://book.getfoundry.sh/reference/cast/cast-wallet-new) to generate a wallet to perform the funding.

```
$ cast wallet new
Successfully created new keypair.
Address:     0xAA15c5cD2005A4534b8593a3e575d8539b9A17dc
Private key: 0x1234
```

Ubiq Bridge Funder address: [0xAA15c5cD2005A4534b8593a3e575d8539b9A17dc](https://ubiqscan.io/address/0xaa15c5cd2005a4534b8593a3e575d8539b9a17dc)

### Set up Environment variables

Copy the sample file and modify

```
cp env.sample env
```

Use Docker or a wrapper run script.
