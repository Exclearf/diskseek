package indexfile

type PostingsCodec uint32

const (
	PostingsCodecRaw   PostingsCodec = 1
	PostingsCodecVByte PostingsCodec = 2
)

func (codec PostingsCodec) supported() bool {
	return codec == PostingsCodecRaw || codec == PostingsCodecVByte
}
