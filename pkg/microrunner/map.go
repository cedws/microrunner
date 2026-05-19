package microrunner

import "sync"

type syncMap[T any, U any] struct {
	m sync.Map
}

func (m *syncMap[T, U]) Len() int {
	len := 0
	m.m.Range(func(key, value any) bool {
		len++
		return true
	})
	return len
}

func (m *syncMap[T, U]) Iter() func(func(T, U) bool) {
	return func(f func(T, U) bool) {
		m.m.Range(func(key, value any) bool {
			return f(key.(T), value.(U))
		})
	}
}

func (m *syncMap[T, U]) Store(key T, value U) {
	m.m.Store(key, value)
}

func (m *syncMap[T, U]) Load(key T) (U, bool) {
	var zero U
	val, ok := m.m.Load(key)
	if !ok {
		return zero, false
	}
	return val.(U), true
}

func (m *syncMap[T, U]) Delete(key T) {
	m.m.Delete(key)
}
