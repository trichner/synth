package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/hajimehoshi/oto"
	"sync"
)

const (
	freqC = 523.3
	freqE = 659.3
	freqG = 784.0
)

func main() {
	// Prepare an Oto context (this will use your default audio device) that will
	// play all our sounds. Its configuration can't be changed later.

	// Usually 44100 or 48000. Other values might cause distortions in Oto
	samplingRate := 48000

	// Number of channels (aka locations) to play sounds from. Either 1 or 2.
	// 1 is mono sound, and 2 is stereo (most speakers are stereo).
	numOfChannels := 1

	// Bytes used by a channel to represent one sample. Either 1 or 2 (usually 2).
	audioBitDepth := 2

	// Buffer size
	bufferSize := 1024

	// Remember that you should **not** create more than one context
	otoCtx, err := oto.NewContext(samplingRate, numOfChannels, audioBitDepth, bufferSize)
	if err != nil {
		panic("Failed to create oto context")
	}

	// Create a new 'player' that will handle our sound. Paused by default.
	player := otoCtx.NewPlayer()
	defer player.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		saw := &SawToothWave{
			pos:    0,
			length: samplingRate * 4,
			period: int(float64(samplingRate)/freqC + 0.5),
		}
		saw2 := &Amplifier{
			amplification: 0.1,
			signal:        saw,
		}
		echo := &Echo{
			signal:        saw2,
			buf:           make([]uint16, 3939),
			amplification: 0.5,
		}
		square := &SquareWave{
			pos:    0,
			length: samplingRate * 2,
			period: int(float64(samplingRate)/freqG + 0.5),
		}
		square2 := &Amplifier{
			amplification: 0.1,
			signal:        square,
		}
		squareB := &SquareWave{
			pos:    0,
			length: samplingRate * 2,
			period: int(float64(samplingRate)/freqE + 0.5),
		}
		squareB2 := &Amplifier{
			amplification: 0.1,
			signal:        squareB,
		}

		wave := &Mixer{signals: []SampleReader{
			echo,
			&Sequence{
				sequences: []SampleReader{
					square2,
					squareB2,
				}}},
		}

		buf := make([]byte, 2)
		for {
			sample, err := wave.Read()
			if errors.Is(err, ErrEndOfSamples) {
				return
			}
			if err != nil {
				panic(err)
			}
			binary.LittleEndian.PutUint16(buf, sample)
			_, err = player.Write(buf)
			if err != nil {
				panic(err)
			}
		}
	}()

	wg.Wait()
}

var ErrEndOfSamples = fmt.Errorf("end of samples")

type SampleReader interface {
	Read() (uint16, error)
}

type Echo struct {
	signal        SampleReader
	pos           int
	buf           []uint16
	amplification float64
}

func (e *Echo) Read() (uint16, error) {
	s, err := e.signal.Read()
	if err != nil {
		return 0, err
	}

	pos := e.pos % len(e.buf)
	e.pos++

	old := e.buf[pos]
	e.buf[pos] = s

	old = uint16(float64(old) * e.amplification)
	return s + old, nil
}

type Sequence struct {
	sequences []SampleReader
	pos       int
}

func (s *Sequence) Read() (uint16, error) {
	if s.pos == len(s.sequences) {
		return 0, ErrEndOfSamples
	}

	seq := s.sequences[s.pos]

	sample, err := seq.Read()
	for errors.Is(err, ErrEndOfSamples) {
		s.pos++
		return s.Read()
	}
	return sample, nil
}

type Mixer struct {
	signals []SampleReader
}

func (m *Mixer) Read() (uint16, error) {

	var sample uint16
	var anySignal bool
	for _, signal := range m.signals {
		s, err := signal.Read()
		if err == ErrEndOfSamples {
			continue
		}
		sample += s
		anySignal = true
	}
	if !anySignal {
		return 0, ErrEndOfSamples
	}

	return sample, nil
}

type Amplifier struct {
	amplification float64
	signal        SampleReader
}

func (a *Amplifier) Read() (uint16, error) {

	sample, err := a.signal.Read()
	if err != nil {
		return 0, err
	}

	sample = uint16(float64(sample) * a.amplification)
	return sample, nil
}

type SawToothWave struct {
	pos    int
	length int
	period int
}

func (s *SawToothWave) Read() (uint16, error) {

	if s.pos == s.length {
		return 0, ErrEndOfSamples
	}

	t := float64(s.pos%s.period) / float64(s.period)
	sample := uint16(0xFFFF * t)

	s.pos++
	return sample, nil
}

type SquareWave struct {
	pos    int
	length int
	period int
}

func (s *SquareWave) Read() (uint16, error) {
	if s.pos == s.length {
		return 0, ErrEndOfSamples
	}

	on := (s.pos/s.period)%2 == 0
	s.pos++

	if on {
		return 0xFFFF, nil
	} else {
		return 0, nil
	}
}
