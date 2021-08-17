module github.com/palettechain/palette-relayer

go 1.14

require (
	github.com/boltdb/bolt v1.3.1
	github.com/btcsuite/btcd v0.20.1-beta
	github.com/ethereum/go-ethereum v1.9.15
	github.com/ontio/ontology-crypto v1.0.9
	github.com/polynetwork/eth-contracts v0.0.0-20200814062128-70f58e22b014
	github.com/polynetwork/poly v1.7.2
	github.com/polynetwork/poly-go-sdk v0.0.0-20200730112529-d9c0c7ddf3d8
	github.com/polynetwork/poly-io-test v0.0.0-20200819093740-8cf514b07750 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli v1.22.4
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
)

replace github.com/ethereum/go-ethereum v1.9.15 => github.com/palettechain/palette v0.2.3-0.20210114015914-c9a5d0b2d92c
