package lfu

import (
	"reflect"
	"testing"
)

type String string

func (d String) Len() int {
	return len(d)
}

func TestGet(t *testing.T) {
	lfu := New(int64(0), nil, 60)
	//在这个特定的上下文中，int64(0) 作为参数传递给 New 函数，用于指定 LRU 缓存的最大存储容量。
	//在这里，将其设置为 0 表示缓存的最大容量为零，即没有存储空间，因此不会保存任何键值对。
	//这可以用于创建一个非常小的缓存或用于特定的测试场景，其中不需要实际存储数据。
	lfu.Add("key1", String("1234"), 60)
	if v, ok := lfu.Get("key1"); !ok || string(v.(String)) != "1234" {
		t.Fatalf("cache hit key1=1234 failed")
	}
	if _, ok := lfu.Get("key2"); ok {
		t.Fatalf("cache miss key2 failed")
	}
}

func TestRemoveoldest(t *testing.T) {
	k1, k2, k3 := "key1", "key2", "k3"
	v1, v2, v3 := "value1", "value2", "v3"
	Cap := len(k1 + k2 + v1 + v2)
	lfu := New(int64(Cap), nil, 60)
	lfu.Add(k1, String(v1), 60)
	lfu.Add(k2, String(v2), 60)
	lfu.Add(k3, String(v3), 60)

	if _, ok := lfu.Get("key1"); ok || lfu.Len() != 2 {
		t.Fatalf("Removeoldest key1 failed")
	}
}

func TestOnEvicted(t *testing.T) {
	keys := make([]string, 0)
	callback := func(key string, value Value) {
		keys = append(keys, key)
	}
	lfu := New(int64(10), callback, 60)
	lfu.Add("key1", String("123456"), 60)
	lfu.Add("k2", String("k2"), 60)
	lfu.Add("k3", String("k3"), 60)
	lfu.Add("k4", String("k4"), 60)
	expect := []string{"key1", "k2"}
	if !reflect.DeepEqual(expect, keys) {
		t.Fatalf("Call onEvicted failed,expect keys equals to %s", expect)
	}
}

func TestAdd(t *testing.T) {
	lfu := New(int64(0), nil, 60)
	lfu.Add("key", String("1"), 60)
	lfu.Add("key", String("111"), 60)

	if lfu.nBytes != int64(len("key")+len("111")) {
		t.Fatal("expected 6 but got", lfu.nBytes)
	}
}
