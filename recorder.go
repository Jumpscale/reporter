package reporter

//Recorder interface
type Recorder interface {
	Record(blk *Block) error
	Close() error
}
