package blindbaseline

import (
	"reflect"
	"strings"
	"testing"
)

func TestBindingFieldRegistryCoversJSONContractInOrder(t *testing.T) {
	typeOfBinding := reflect.TypeOf(Binding{})
	fields := (Binding{}).fields()
	if len(fields) != typeOfBinding.NumField() {
		t.Fatalf("binding registry fields = %d, struct fields = %d", len(fields), typeOfBinding.NumField())
	}
	seen := map[string]bool{}
	for index := range fields {
		name := strings.Split(typeOfBinding.Field(index).Tag.Get("json"), ",")[0]
		if name == "" || name != fields[index].name || seen[name] {
			t.Fatalf("binding registry entry %d does not match unique JSON field %q", index, name)
		}
		seen[name] = true
		if fields[index].kind < bindingCommit || fields[index].kind > bindingVersion {
			t.Fatalf("binding registry entry %q has invalid validation kind", name)
		}
	}
}
