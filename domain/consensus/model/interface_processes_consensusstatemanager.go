package model

import "github.com/kaspanet/kaspad/domain/consensus/model/externalapi"

// ConsensusStateManager manages the node's consensus state
type ConsensusStateManager interface {
	AddBlockToVirtual(blockHash *externalapi.DomainHash) error
	PopulateTransactionWithUTXOEntries(transaction *externalapi.DomainTransaction) error
	SetPruningPointUTXOSet(pruningPoint *externalapi.DomainHash, serializedUTXOSet []byte) error
}