package unrelatedproducer

// A same-named function as producer.GetLdClient(), but returning a
// completely unrelated type — proves factory resolution keys off the
// declared return type actually matching the SDK client, not off the
// function name, and off a real, followed import, not a name coincidence
// across unrelated packages.
type FakeClient struct{}

func (c *FakeClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func GetLdClient() *FakeClient {
	return &FakeClient{}
}
