package xair

import "fmt"

func argAt(args []interface{}, addr string, i int) (interface{}, error) {
	if i >= len(args) {
		return nil, fmt.Errorf("xair: %s: expected at least %d argument(s), got %d", addr, i+1, len(args))
	}
	return args[i], nil
}

func argFloat32(args []interface{}, addr string) (float32, error) {
	v, err := argAt(args, addr, 0)
	if err != nil {
		return 0, err
	}
	switch t := v.(type) {
	case float32:
		return t, nil
	case int32:
		return float32(t), nil
	default:
		return 0, fmt.Errorf("xair: %s: unexpected reply type %T", addr, v)
	}
}

func argInt32(args []interface{}, addr string) (int32, error) {
	v, err := argAt(args, addr, 0)
	if err != nil {
		return 0, err
	}
	switch t := v.(type) {
	case int32:
		return t, nil
	case float32:
		return int32(t), nil
	default:
		return 0, fmt.Errorf("xair: %s: unexpected reply type %T", addr, v)
	}
}

func argString(args []interface{}, addr string) (string, error) {
	v, err := argAt(args, addr, 0)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("xair: %s: unexpected reply type %T", addr, v)
	}
	return s, nil
}
