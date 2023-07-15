package geecache

type ByteView struct {
	b []byte //b 将会存储真实的缓存值。选择 byte 类型是为了能够支持任意的数据类型的存储，例如字符串、图片等。
}

func (v ByteView) Len() int {
	return len(v.b)
} //实现 Len() int 方法，我们在 lru.Cache 的实现中，要求被缓存对象必须实现 Value 接口，即 Len() int 方法，返回其所占的内存大小。

func (v ByteView) ByteSlice() []byte {
	return cloneBytes(v.b)
} //b 是只读的，使用 ByteSlice() 方法返回一个拷贝，防止缓存值被外部程序修改。

func (v ByteView) String() string {
	return string(v.b)
}

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
