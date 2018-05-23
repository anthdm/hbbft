# hbbft
Practical implementation of the Honey Badger Byzantine Fault Tolerance consensus algorithm written in Go.

<p align="center">
  <a href="https://github.com/anthdm/hbbft/releases">
    <img src="https://img.shields.io/github/tag/anthdm/hbbft.svg?style=flat">
  </a>
  <a href="https://circleci.com/gh/anthdm/hbbft/tree/master">
    <img src="https://circleci.com/gh/anthdm/hbbft/tree/master.svg?style=shield">
  </a>
  <a href="https://goreportcard.com/report/github.com/anthdm/hbbft">
    <img src="https://goreportcard.com/badge/github.com/anthdm/hbbft">
  </a>
</p>

### Summary
This package includes the building blocks for implementing a practical version of the hbbft protocol. The exposed engine can be plugged easily into existing applications. Users can choose to use the transport layer or roll their own. For implementation details take a look at the simulations which implements hbbft into a realistic scenario. The building blocks of hbbft exist out of the following sub-protocols: 

#### Reliable broadcast (RBC)
Uses reedsolomon erasure encoding to disseminate an ecrypted set of transactions.

#### Binary Byzantine Agreement (BBA)
Uses a common coin to agree that a majority of the participants have a consensus that RBC has completed. 

#### Asynchronous Common Subset (ACS)
Combines RBC and BBA to agree on a set of encrypted transactions.

#### HoneyBadger
Top level HoneyBadger protocol that implements all the above sub(protocols) into a complete --production grade-- practical consensus engine. 

### Usage
Install dependencies
```
make deps
```

Run tests
```
make test
```

### Current project state
- [x] Reliable Broadcast Algorithm
- [x] Binary Byzantine Agreement
- [x] Asynchronous Common Subset 
- [x] HoneyBadger top level protocol 

### TODO
- [ ] Treshold encryption
- [ ] Configurable serialization for transactions 

### References
- [The Honey Badger BFT protocols](https://eprint.iacr.org/2016/199.pdf)
- [Practical Byzantine Fault Tolerance](http://pmg.csail.mit.edu/papers/osdi99.pdf)
- [Treshold encryption](https://en.wikipedia.org/wiki/Threshold_cryptosystem)
- [Shared secret](https://en.wikipedia.org/wiki/Shared_secret)

### Other language implementations
- [Rust](https://github.com/poanetwork/hbbft)
- [Erlang](https://github.com/helium/erlang-hbbft)
