package lzss

import (
	"bytes"
	"errors"
	"github.com/consensys/gnark/std/compress"
)

// The backref logic can produce RLE as a special case, which is good for decompressor state machine complexity
// however we have to make some sacrifices such as allowing very small lengths/offsets that wouldn't be viable for a "real" backref
// if the state machine logic turned out to be a tiny portion of total decompressor constraints, as it's expected to be,
// consider separating RLE/backref logics

// Compress applies a DEFLATE-inspired, LZSS-type compression on d.
// It does well on data with many long repeated substrings and long runs of similar bytes, e.g. programmatic data.
// It can be improved by further compression using a prefix-free code, such as Huffman coding.
// In fact, DEFLATE is LZSS + Huffman coding. It is used by gzip which is the standard tool for compressing programmatic data.
// For more information, refer to Bill Bird's fantastic undergraduate course on Data Compression
// In particular those on the LZ family: https://youtu.be/z1I1o7zySUI and DEFLATE: https://youtu.be/SJPvNi4HrWQ
func Compress(d []byte, settings Settings) (c []byte, err error) {
	// d[i < 0] = settings.BackRefSettings.Symbol by convention
	var out bytes.Buffer

	if settings.ReferenceTo == Compressed {
		return nil, errors.New("compressed ref not implemented")
	}
	if settings.AddressingMode == Absolute {
		return nil, errors.New("absolute addressing not implemented")
	}
	if settings.Log {
		return nil, errors.New("logging not implemented")
	}

	// we write offset first and then length, for no particular reason
	// "nontrivial" meaning of length and offset more than 1
	minNontrivialBackRefCost := int(compress.ByteGasCost(settings.Symbol)) + 8 + int(settings.NbBytesAddress+settings.NbBytesLength-2)
	// any string of lesser cost than minBackrefCost is not worth compressing
	backRefAddressRange := 1 << (settings.NbBytesAddress * 8)
	backRefLengthRange := 1 << (settings.NbBytesLength * 8)
	emitBackRef := func(offset, length int) {
		emit(&out, offset-1, settings.NbBytesAddress)
		emit(&out, length-1, settings.NbBytesLength)
	}
	// this also means that very short runs of zeros are expanded rather than compressed
	// TODO replace this with a "dynamic" gas-cost related heuristic

	getRunLength := func(i int, cap int) int {
		if cap == -1 {
			cap = len(d)
		}
		j := i + 1
		for j < len(d) && j < cap && d[j] == settings.Symbol {
			j++
		}
		return j - i
	}

	i := 0
	for i < len(d) {
		// if there is a run of the character used to mark backrefs, we have to make a backref regardless of whether it achieves compression
		if d[i] == settings.Symbol {

			maxJExpressible := i + 1 + backRefLengthRange
			runLength := getRunLength(i, maxJExpressible) // TODO If logging, go past maxJExpressible to spot missed opportunities
			// making a "back reference" to negative indices
			if i == 0 { // "back reference" the stream itself as it is being written
				emitBackRef(1, runLength)
			} else if
			// TODO Limit the negative index idea to only -1 and add an explicit rule to handle it in the decompressor if the extra 1 << NbLengthBytes table entries were too expensive
			i <= backRefAddressRange { // TODO make sure the boundary is correct
				emit(&out, i, settings.BackRefSettings.NbBytesAddress)
				emit(&out, runLength-1, settings.BackRefSettings.NbBytesLength)
			} else {
				// no access to negative indices, so we have to find actual backrefs
				// TODO cache the symb-run backrefs?
				for runLength > 0 {
					longestRunLen := 0
					longestRunStartIndex := 0
					currentRunLength := 0

					for k := i - 1; k >= 0; k-- {
						if d[k] == settings.Symbol {
							currentRunLength++
						} else {
							currentRunLength = 0
						}
						if currentRunLength > longestRunLen {
							longestRunLen = currentRunLength
							longestRunStartIndex = k
							if currentRunLength == runLength {
								break
							}
						}
					}
					if currentRunLength == 0 {
						return nil, errors.New("no backref found")
					}
					emitBackRef(i-longestRunStartIndex, longestRunLen)
				}

				// TODO Find as many actual backrefs as needed
				return nil, errors.New("not yet implemented")
			}
			i += runLength
		} else {
			// attempt to find a backref, if it's worthwhile
			minViableBackRefLength := 2
			noBackRefCost := int(compress.ByteGasCost(d[i]))
			var midRle bool
			for {
				// not even trying to factor in the cost of RLE TODO: that
				if i+minViableBackRefLength > len(d) {
					minViableBackRefLength = -1 // just not viable
					break
				}

				curr := d[i+minViableBackRefLength-1]

				if curr == settings.Symbol {
					midRle = true
				} else {
					if midRle {
						noBackRefCost += minViableBackRefLength // getting rid of an RLE, though the cost is not exact. TODO: fix that (probably move the RLE logic to a separate function that could be called here)
					}
					midRle = false

					noBackRefCost += int(compress.ByteGasCost(d[i+minViableBackRefLength-1]))
					if noBackRefCost >= minNontrivialBackRefCost {
						break
					}
				}
				minViableBackRefLength++
			}

			if addr, length := longestMostRecentBackRef(d, i, i-backRefAddressRange-1, minViableBackRefLength); length != -1 {
				emitBackRef(i-addr-1, length-1)
				i += length
			} else {
				// no backref found
				out.WriteByte(d[i])
				i++
			}
		}
	}

	return out.Bytes(), nil
}

func longestMostRecentBackRef(d []byte, i int, minBackRefAddr, minViableBackRefLen int) (addr, length int) {
	// TODO: Implement an efficient string search algorithm
	// greedily find the longest backref with smallest offset TODO better heuristic?
	minViableBackRef := d[i : i+minViableBackRefLen]
	var remainingOptions map[int]struct{}
	for j := i - 1; j >= 0 && j >= minBackRefAddr; j-- { // TODO If logging is enabled, go past minBackRefAddr to spot missed opportunities
		if j+minViableBackRefLen > len(d) {
			continue
		}
		if bytes.Equal(d[j:j+minViableBackRefLen], minViableBackRef) {
			remainingOptions[j] = struct{}{}
		}
	}
	var toDelete []int
	l := minViableBackRefLen
	for ; len(toDelete) < len(remainingOptions); l++ {
		for _, j := range toDelete {
			delete(remainingOptions, j)
		}
		toDelete = toDelete[:0]
		for j := range remainingOptions {
			if j+l > len(d) || d[j+l] != d[i+l] {
				toDelete = append(toDelete, j)
			}
		}
	}
	if len(remainingOptions) == 0 {
		return -1, -1
	}
	mostRecent := toDelete[0]
	for _, j := range toDelete {
		if j > mostRecent {
			mostRecent = j
		}
	}
	return mostRecent, l
}

// emit writes little endian
func emit(bb *bytes.Buffer, n int, nbBytes uint) {
	for i := uint(0); i < nbBytes; i++ {
		bb.WriteByte(byte(n))
		n >>= 8
	}
	if n != 0 {
		panic("n does not fit in nbBytes")
	}
}
