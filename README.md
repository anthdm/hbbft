# hbbft
Practical implementation of the Honey Badger Byzantine Fault Tolerance consensus algorithm written in Go.

### Summary
This package includes the building blocks for implementing a practical version of the hbbft protocol. The exposed engine can be plugged easily into existing applications. For implementation details take a look at the simulations which implements hbbft into a realistic scenario. The building blocks of hbbft exist out of the following sub-protocols: 

#### Reliable broadcast (RBC)
Uses reedsolomon erasure encoding to disseminate an ecrypted set of transactions.

#### Binary Byzantine Agreement (BBA)
Uses a common coin to agree that a majority of the participants have a consensus that RBC has completed. 

#### Asynchronous Common Subset (ACS)
Combines RBC and BBA to agree on a set of encrypted transactions.

### Usage
Install dependencies
```
make deps
```

Run tests
```
make test
```

### References
- [The Honey Badger BFT protocols](https://eprint.iacr.org/2016/199.pdf)
- [Practical Byzantine Fault Tolerance](http://pmg.csail.mit.edu/papers/osdi99.pdf)
- [Treshold encryption](https://en.wikipedia.org/wiki/Threshold_cryptosystem)
- [Shared secret](https://en.wikipedia.org/wiki/Shared_secret)

