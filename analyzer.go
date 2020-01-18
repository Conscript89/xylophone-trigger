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

type DisplaySettings struct {
	maxValue float32
	from int
	minFrom int
	to int
	maxTo int
}

func (settings *DisplaySettings) init(to int) {
	settings.maxValue = 100
	settings.minFrom = 1 // skip DC part
	settings.from = settings.minFrom
	settings.maxTo = to
	settings.to = settings.maxTo
}

func (settings *DisplaySettings) zoomY(scale float32) {
	settings.maxValue *= scale
}

func (settings *DisplaySettings) zoomX(scale float32) {
	// FIXME: crashing when shifted
	diff := settings.to - settings.from
	diff = (int)((float32)(diff) / scale)
	// diff must be bigger then 3 and smaller then max diff
	diff = (int)(math.Max((float64)(diff), 3))
	diff = (int)(math.Min((float64)(diff), (float64)(settings.maxTo - settings.minFrom)))
	// apply the scale
	settings.to = settings.from + diff
}

func (settings *DisplaySettings) shiftX(relativeStep float32) {
	diff := settings.to - settings.from
	if relativeStep > 0 {
		settings.to += (int)((float32)(diff)/relativeStep)
		settings.to = (int)(math.Min((float64)(settings.to), (float64)(settings.maxTo)))
		settings.from = settings.to - diff
	} else {
		settings.from += (int)((float32)(diff)/relativeStep)
		settings.from = (int)(math.Max((float64)(settings.from), (float64)(settings.minFrom)))
		settings.to = settings.from + diff
	}
}

type Gui struct {
	width int32
	height int32
	window *sdl.Window
	surface *sdl.Surface
}

func (gui *Gui) clear() {
	everything := sdl.Rect{0, 0, gui.width, gui.height}
	gui.surface.FillRect(
		&everything,
		sdl.Color{255, 0, 0, 0}.Uint32(),
	)
}

func (gui *Gui) drawBar(data *AggregatedData, bars int, index int, maxValue float32, value float32, isPeak bool) {
	var barRect sdl.Rect
	barColor := sdl.Color{255, 255, 0, 0}.Uint32()
	barWidth := gui.width / (int32)(bars)
	barHeight := (value / maxValue) * (float32)(gui.height)
	barRect.X = (int32)(index) * barWidth
	barRect.Y = gui.height - (int32)(barHeight)
	barRect.W = barWidth
	barRect.H = (int32)(barHeight)
	gui.surface.FillRect(&barRect, barColor)
}

func (gui *Gui) drawBars(data *AggregatedData, displaySettings DisplaySettings) {
	from := displaySettings.from
	to := displaySettings.to
	bars := to - from
	for i := from; i < to; i++ {
		gui.drawBar(
			data, bars, i-from,
			(float32)(displaySettings.maxValue), data.values[i],
			false,
		)
	}
}

func (gui *Gui) drawScales(displaySettings DisplaySettings) {
	var scales = []int32{100, 10, 5, 2}
	xDiff := displaySettings.to - displaySettings.from
	var scale int32
	var scaleRect sdl.Rect
	scaleColor := sdl.Color{255, 255, 255, 0}.Uint32()
	for _, potentialScale := range scales {
		scale = potentialScale
		if (int32)(xDiff) / potentialScale > 3 {
			break
		}
	}
	scaleRect.Y = 0
	scaleRect.W = gui.width / (int32)(xDiff)
	scaleRect.H = gui.height
	for x := -((int32)(displaySettings.from) % scale); x <= (int32)(xDiff); x += scale {
		scaleRect.X = x * scaleRect.W
		gui.surface.FillRect(&scaleRect, scaleColor)
	}
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
		gui.surface, error = gui.window.GetSurface()
	}
	error = sdl.Init(sdl.INIT_AUDIO)
	print_error(error)
	error = recordData.openRecordDevice(options)
	print_error(error)
}

func mainloop(options Options, gui *Gui) {
	var displaySettings DisplaySettings
	running := true
	capturing := true
	currentData := new(AggregatedData)
	// initial display configuration
	displaySettings.init(options.samples/2-1)
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
				switch t.State {
				case sdl.RELEASED:
					switch t.Keysym.Sym {
					case sdl.K_UP:
						displaySettings.zoomY(0.75)
					case sdl.K_DOWN:
						displaySettings.zoomY(1.25)
					case sdl.K_EQUALS:
						fallthrough
					case sdl.K_KP_PLUS:
						displaySettings.zoomX(2)
					case sdl.K_MINUS:
						fallthrough
					case sdl.K_KP_MINUS:
						displaySettings.zoomX(0.5)
					case sdl.K_RIGHT:
						displaySettings.shiftX(10)
					case sdl.K_LEFT:
						displaySettings.shiftX(-10)
					case sdl.K_ESCAPE:
						fallthrough
					case sdl.K_q:
						running = false
					case sdl.K_SPACE:
						capturing = !capturing
						sdl.PauseAudioDevice(recordData.device, !capturing)
					default:
						fmt.Fprintf(os.Stderr, "Unhanled key: '%s'\n", string(t.Keysym.Sym))
					}
					break
				}
				break
			}
		}
		currentData.update(recordData)
		if options.debug {
			gui.clear()
			gui.drawScales(displaySettings)
			gui.drawBars(currentData, displaySettings)
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
	var gui Gui
	gui.width = 2048
	gui.height = 1000
	parseArgs(&options)
	fmt.Printf("Options: %v\n", options)
	recordData = new(AudioData)
	recordData.init(options)
	init_sdl(options, &gui)
	mainloop(options, &gui)
	sdl.Quit()
}
