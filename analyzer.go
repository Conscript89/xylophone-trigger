package main

/*
typedef unsigned char Uint8;
void recordCallback(void* userdata, Uint8* stream, int len);
*/
import "C"

import (
	"os"
	"errors"
	"flag"
	"fmt"
	"unsafe"
	"sync"
	"math"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/jvlmdr/go-fftw/fftw"
)

type Options struct {
	debug bool
	tune bool
	frequency int
	samples int
	historySize int
	interval int
}

type Gui struct {
	width int32
	height int32
	window *sdl.Window
}

func (gui *Gui) clear() {
	everything := sdl.Rect{0, 0, gui.width, gui.height}
	surface, _ := gui.window.GetSurface()
	surface.FillRect(
		&everything,
		sdl.Color{255, 0, 0, 0}.Uint32(),
	)
}

func (gui *Gui) drawBars(data *AggregatedData) {
	gui.window.UpdateSurface()
}

func (gui *Gui) flip() {
	gui.window.UpdateSurface()
}

type AggregatedData struct {
	values []float32
}

func (data *AggregatedData) init(options Options) {
	data.values = make([]float32, options.samples/2)
}

func (data *AggregatedData) update(src *AudioData) {
	// locks
	recordData.mux.Lock()
	defer recordData.mux.Unlock()
	// process
	for i, _ := range(data.values) {
		data.values[i] = src.avgMagnitudeAt(i)
	}
}

type AudioData struct {
	mux sync.Mutex
	values[] *fftw.Array
	size int
	counter int
	device sdl.AudioDeviceID
}

func (data *AudioData) init(options Options) {
	data.size = options.historySize
	data.counter = 0
	data.values = make([]*fftw.Array, data.size)
	for i := 0; i < data.size; i++ {
		data.values[i] = fftw.NewArray(options.samples)
	}
}

const dataFormat = sdl.AUDIO_F32SYS
const dataByteSize = 4
func (data *AudioData) openRecordDevice(options Options) error {
	var want, have sdl.AudioSpec
	var error error
	if data.device != 0 {
		return errors.New("Device is already open.")
	}
	want.Freq = (int32)(options.frequency)
	want.Format = dataFormat
	want.Channels = 1
	want.Samples = (uint16)(options.samples)
	want.Callback = sdl.AudioCallback(C.recordCallback)
	want.UserData = nil
	data.device, error = sdl.OpenAudioDevice("", true, &want, &have, 0)
	return error
}

func (data *AudioData) sumMagnitudeAt(index int) float32 {
	var sum float64 = 0
	for j := 0; j < data.size; j++ {
		sum += magnitude(data.values[j].Elems[index])
	}
	return (float32)(sum)
}

func (data *AudioData) avgMagnitudeAt(index int) float32 {
	return data.sumMagnitudeAt(index) / (float32)(data.size)
}

//export recordCallback
func recordCallback(userdata unsafe.Pointer, stream *C.Uint8, length C.int) {
	// locks
	recordData.mux.Lock()
	defer recordData.mux.Unlock()
	// continue with code
	index := recordData.counter % recordData.size
	dataSlice := (*[1<<30]float32)(unsafe.Pointer(stream))[:length/dataByteSize:length/dataByteSize]
	for i := 0; i < (int)(length/dataByteSize); i++ {
		recordData.values[index].Elems[i] = (complex128)(complex(dataSlice[i], 0))
	}
	fftw.FFTTo(recordData.values[index], recordData.values[index])
	recordData.counter++
	//fmt.Printf("Data: %v\n", recordData.values.Elems)
	//fmt.Printf("Test: %v\n", recordData.counter)
}

func magnitude(item complex128) float64 {
	return math.Sqrt((float64)(real(item)*real(item) + imag(item)*imag(item)))
}

func parseArgs(options *Options) {
	flag.BoolVar(
		&options.debug, "debug", false, "turn on debug mode",
	)
	flag.BoolVar(
		&options.tune, "tune", false, "turn on tuning mode",
	)
	flag.IntVar(
		&options.frequency, "frequency", 44100, "Sound capture frequency",
	)
	flag.IntVar(
		&options.samples, "samples", 2048, "Number of samples captured",
	)
	flag.IntVar(
		&options.historySize, "history-size", 3,
		"Number of previous results taken in account",
	)
	flag.IntVar(
		&options.interval, "interval", 10, "Analyze and draw interval",
	)
	flag.Parse()
}

func print_error(error error) {
	if error != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", error)
	}
}

func init_sdl(options Options, gui *Gui) {
	var error error
	if options.debug {
		error = sdl.Init(sdl.INIT_VIDEO)
		print_error(error)
		gui.window, error = sdl.CreateWindow(
			"analyzer", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED,
			gui.width, gui.height, 0,
		)
		print_error(error)
	}
	error = sdl.Init(sdl.INIT_AUDIO)
	print_error(error)
	error = recordData.openRecordDevice(options)
	print_error(error)
}

func mainloop(options Options, gui *Gui) {
	running := true
	capturing := true
	currentData := new(AggregatedData)
	// start capturing data
	currentData.init(options)
	sdl.PauseAudioDevice(recordData.device, !capturing)
	for running {
		sdl.PumpEvents()
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch t := event.(type) {
			case *sdl.QuitEvent:
				running = false
				break
			case *sdl.KeyboardEvent:
				keyCode := t.Keysym.Sym
				switch t.State {
				case sdl.RELEASED:
					switch string(keyCode) {
					case "q":
						running = false
						break
					case " ":
						capturing = !capturing
						sdl.PauseAudioDevice(recordData.device, !capturing)
					}
					break
				}
				break
			}
		}
		currentData.update(recordData)
		if options.debug {
			gui.clear()
			gui.drawBars(currentData)
			gui.flip()
		} else {
			
		}
		sdl.Delay((uint32)(options.interval))
	}
	// stop capturing data
	if capturing {
		sdl.PauseAudioDevice(recordData.device, true)
	}
	fmt.Printf("END LOOP\n")
}

var recordData *AudioData

func main() {
	var options Options
	var gui Gui = Gui{2048, 1000, nil}
	parseArgs(&options)
	fmt.Printf("Options: %v\n", options)
	recordData = new(AudioData)
	recordData.init(options)
	init_sdl(options, &gui)
	mainloop(options, &gui)
	sdl.Quit()
}
