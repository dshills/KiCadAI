package blocks

import (
	"fmt"

	"kicadai/internal/transactions"
)

const DefaultGeneratedProjectName = "generated_blocks"

func ProjectTransactionForBlockOutput(projectName string, output BlockOutput, overwrite bool) (transactions.Transaction, error) {
	refs := map[string]struct{}{}
	for _, ref := range output.Instance.Refs {
		refs[ref] = struct{}{}
	}
	return projectTransaction(projectName, output.Operations, refs, overwrite)
}

func ProjectTransactionForCompositionOutput(projectName string, output CompositionOutput, overwrite bool) (transactions.Transaction, error) {
	refs := map[string]struct{}{}
	for _, instance := range output.Instances {
		for _, ref := range instance.Refs {
			refs[ref] = struct{}{}
		}
	}
	return projectTransaction(projectName, output.Operations, refs, overwrite)
}

func projectTransaction(projectName string, operations []transactions.Operation, generatedRefs map[string]struct{}, overwrite bool) (transactions.Transaction, error) {
	if projectName == "" {
		projectName = DefaultGeneratedProjectName
	}
	create, err := wrapOperation(transactions.OpCreateProject, transactions.CreateProjectOperation{Op: transactions.OpCreateProject, Name: projectName})
	if err != nil {
		return transactions.Transaction{}, err
	}
	write, err := wrapOperation(transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject, Overwrite: overwrite})
	if err != nil {
		return transactions.Transaction{}, err
	}
	tx := transactions.Transaction{Name: projectName, Project: projectName, Operations: []transactions.Operation{create}}
	for _, operation := range operations {
		if operation.Op == transactions.OpConnect {
			generated, err := connectEndpointsAreGenerated(operation, generatedRefs)
			if err != nil {
				return transactions.Transaction{}, err
			}
			if !generated {
				continue
			}
		}
		tx.Operations = append(tx.Operations, operation)
	}
	tx.Operations = append(tx.Operations, write)
	return tx, nil
}

func connectEndpointsAreGenerated(operation transactions.Operation, generatedRefs map[string]struct{}) (bool, error) {
	var payload transactions.ConnectOperation
	if err := decodeBlockOperation(operation, &payload); err != nil {
		return false, fmt.Errorf("decode connect operation: %w", err)
	}
	_, fromOK := generatedRefs[payload.From.Ref]
	_, toOK := generatedRefs[payload.To.Ref]
	return fromOK && toOK, nil
}
