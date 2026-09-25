package idl

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:embed torch_market.json
var idlJSON []byte

type IDL struct {
	Address      string
	Instructions map[string]Instruction
	Types        map[string][]Field
}

type Instruction struct {
	Name          string
	Discriminator []byte
	Accounts      []Account
	Args          []Arg
}

type Account struct {
	Name     string
	Writable bool
	Signer   bool
}

type Arg struct {
	Name    string
	Type    string
	Defined string
}

type Field struct {
	Name string
	Type string
}

type idlJSONFile struct {
	Address      string           `json:"address"`
	Instructions []idlInstruction `json:"instructions"`
	Types        []idlType        `json:"types"`
}

type idlInstruction struct {
	Name          string       `json:"name"`
	Discriminator []int        `json:"discriminator"`
	Accounts      []idlAccount `json:"accounts"`
	Args          []idlArg     `json:"args"`
}

type idlAccount struct {
	Name     string `json:"name"`
	Writable bool   `json:"writable"`
	Signer   bool   `json:"signer"`
}

type idlArg struct {
	Name string     `json:"name"`
	Type idlArgType `json:"type"`
}

type idlArgType struct {
	Defined string
}

func (t *idlArgType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		t.Defined = s
		return nil
	}
	var obj struct {
		Defined struct {
			Name string `json:"name"`
		} `json:"defined"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	t.Defined = obj.Defined.Name
	return nil
}

type idlType struct {
	Name string     `json:"name"`
	Type idlTypeDef `json:"type"`
}

type idlTypeDef struct {
	Kind   string     `json:"kind"`
	Fields []idlField `json:"fields"`
}

type idlField struct {
	Name string       `json:"name"`
	Type idlFieldType `json:"type"`
}

type idlFieldType struct {
	Prim    string          `json:"prim"`
	Defined json.RawMessage `json:"defined"`
	Vec     *idlFieldType   `json:"vec"`
	Option  *idlFieldType   `json:"option"`
	Array   *idlArray       `json:"array"`
}

type idlArray []json.RawMessage

var parsedIDL *IDL

func LoadIDL() (*IDL, error) {
	if parsedIDL != nil {
		return parsedIDL, nil
	}
	var raw idlJSONFile
	if err := json.Unmarshal(idlJSON, &raw); err != nil {
		return nil, fmt.Errorf("idl: %w", err)
	}
	id := &IDL{
		Address:      raw.Address,
		Instructions: map[string]Instruction{},
		Types:        map[string][]Field{},
	}
	for _, t := range raw.Types {
		fields := make([]Field, 0, len(t.Type.Fields))
		for _, f := range t.Type.Fields {
			fields = append(fields, Field{Name: f.Name, Type: typeName(f.Type)})
		}
		id.Types[t.Name] = fields
	}
	for _, in := range raw.Instructions {
		if len(in.Discriminator) != 8 {
			return nil, fmt.Errorf("idl: instruction %s: discriminator is not 8 bytes", in.Name)
		}
		ins := Instruction{Name: in.Name, Discriminator: make([]byte, 8)}
		for i, b := range in.Discriminator {
			ins.Discriminator[i] = byte(b)
		}
		for _, a := range in.Accounts {
			ins.Accounts = append(ins.Accounts, Account{Name: a.Name, Writable: a.Writable, Signer: a.Signer})
		}
		for _, a := range in.Args {
			ins.Args = append(ins.Args, Arg{Name: a.Name, Type: a.Type.Defined})
		}
		id.Instructions[in.Name] = ins
	}
	parsedIDL = id
	return id, nil
}

func typeName(t idlFieldType) string {
	switch {
	case t.Prim != "":
		return t.Prim
	case len(t.Defined) > 0:
		var d struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(t.Defined, &d); err == nil && d.Name != "" {
			return d.Name
		}
		return string(t.Defined)
	case t.Vec != nil:
		return "vec<" + typeName(*t.Vec) + ">"
	case t.Option != nil:
		return "option<" + typeName(*t.Option) + ">"
	case t.Array != nil:
		return "array"
	default:
		return "unknown"
	}
}

func (id *IDL) BorshArgs(insName string, args map[string]any) ([]byte, error) {
	ins, ok := id.Instructions[insName]
	if !ok {
		return nil, fmt.Errorf("idl: no instruction %q", insName)
	}
	if len(ins.Args) != 1 {
		return nil, fmt.Errorf("idl: %s: expected one args struct, got %d", insName, len(ins.Args))
	}
	structName := ins.Args[0].Type

	if isPrimitive(structName) {
		if len(args) != 1 {
			return nil, fmt.Errorf("idl: %s: flat arg needs one value", insName)
		}
		for _, v := range args {
			return encodeBorsh(structName, v)
		}
	}
	fields, ok := id.Types[structName]
	if !ok {
		return nil, fmt.Errorf("idl: %s: unknown arg type %q", insName, structName)
	}
	var out []byte
	for _, f := range fields {
		v, ok := args[f.Name]
		if !ok {
			return nil, fmt.Errorf("idl: %s: missing arg %s.%s", insName, structName, f.Name)
		}
		b, err := encodeBorsh(f.Type, v)
		if err != nil {
			return nil, fmt.Errorf("idl: %s.%s: %w", structName, f.Name, err)
		}
		out = append(out, b...)
	}
	return out, nil
}

func isPrimitive(t string) bool {
	switch t {
	case "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "bool", "string", "f32", "f64":
		return true
	}
	return false
}

func encodeBorsh(t string, v any) ([]byte, error) {
	switch t {
	case "u64":
		n, ok := v.(uint64)
		if !ok {
			return nil, fmt.Errorf("want u64, got %T", v)
		}
		return LeU64(n), nil
	case "u32":
		n, ok := v.(uint32)
		if !ok {
			return nil, fmt.Errorf("want u32, got %T", v)
		}
		return LeU32(n), nil
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("want bool, got %T", v)
		}
		if b {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case "string":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("want string, got %T", v)
		}
		out := LeU32(uint32(len(s)))
		return append(out, []byte(s)...), nil
	default:
		return nil, fmt.Errorf("unsupported borsh type %q", t)
	}
}

func LeU64(n uint64) []byte {
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(n >> (8 * i))
	}
	return b
}

func LeU32(n uint32) []byte {
	b := make([]byte, 4)
	for i := 0; i < 4; i++ {
		b[i] = byte(n >> (8 * i))
	}
	return b
}

func (id *IDL) Discriminator(name string) ([]byte, error) {
	ins, ok := id.Instructions[name]
	if !ok {
		return nil, fmt.Errorf("idl: no instruction %q", name)
	}
	out := make([]byte, 8)
	copy(out, ins.Discriminator)
	return out, nil
}

func (id *IDL) AccountNames(name string) ([]string, error) {
	ins, ok := id.Instructions[name]
	if !ok {
		return nil, fmt.Errorf("idl: no instruction %q", name)
	}
	out := make([]string, len(ins.Accounts))
	for i, a := range ins.Accounts {
		out[i] = a.Name
	}
	return out, nil
}

func (id *IDL) KnownInstructions() []string {
	out := make([]string, 0, len(id.Instructions))
	for n := range id.Instructions {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (id *IDL) Contains(name string) bool {
	_, ok := id.Instructions[name]
	return ok
}

func (t *idlFieldType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		t.Prim = s
		return nil
	}
	type plain idlFieldType
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*t = idlFieldType(p)
	return nil
}
