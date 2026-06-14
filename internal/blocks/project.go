package blocks

import (
	"fmt"

	"kicadai/internal/transactions"
)

const DefaultGeneratedProjectName = "generated_blocks"

func ProjectTransactionForBlockOutput(projectName string, output BlockOutput, overwrite bool) (transactions.Transaction, error) {
	refs := make(map[string]struct{}, len(output.Instance.Refs))
	for _, ref := range output.Instance.Refs {
		if _, exists := refs[ref]; exists {
			return transactions.Transaction{}, fmt.Errorf("duplicate generated reference %s in instance %s", ref, output.Instance.InstanceID)
		}
		refs[ref] = struct{}{}
	}
	return projectTransaction(projectName, output.Operations, refs, overwrite)
}

func ProjectTransactionForCompositionOutput(projectName string, output CompositionOutput, overwrite bool) (transactions.Transaction, error) {
	refCount := 0
	for _, instance := range output.Instances {
		refCount += len(instance.Refs)
	}
	refs := make(map[string]struct{}, refCount)
	owners := make(map[string]string, refCount)
	for _, instance := range output.Instances {
		for _, ref := range instance.Refs {
			if owner, exists := owners[ref]; exists {
				if owner == instance.InstanceID {
					return transactions.Transaction{}, fmt.Errorf("duplicate generated reference %s within instance %s", ref, owner)
				}
				return transactions.Transaction{}, fmt.Errorf("duplicate generated reference %s in instances %s and %s", ref, owner, instance.InstanceID)
			}
			owners[ref] = instance.InstanceID
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
