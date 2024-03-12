package generate

import (
	"fmt"
	"regexp"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

type Field struct {
	Name     string
	Type     string
	Nullable bool
	Tags     map[string]string
	Column   *plugin.Column
}

var nullableType = regexp.MustCompile(`sql\.Null\[(.*)\]`)

func (self Field) BaseType() string {
	match := nullableType.FindStringSubmatch(self.Type)
	if len(match) > 1 {
		return match[1]
	}
	return self.Type
}

func (self Field) Tag() string {
	return formatTags(self.Tags)
}

func (self Field) HasSlice() bool {
	return self.Column.IsSqlcSlice
}

func checkIncompatibleFieldTypes(fields []Field) error {
	fieldTypes := map[string]string{}
	for _, field := range fields {
		if fieldType, found := fieldTypes[field.Name]; !found {
			fieldTypes[field.Name] = field.Type
		} else if field.Type != fieldType {
			return fmt.Errorf("named param %s has incompatible types: %s, %s", field.Name, field.Type, fieldType)
		}
	}
	return nil
}
