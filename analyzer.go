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
	minPeakValue float64
}

type DisplaySettings struct {
	maxValue float64
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

func (settings *DisplaySettings) bars() int {
	return settings.to - settings.from
}

func (settings *DisplaySettings) zoomY(scale float64) {
	settings.maxValue *= scale
}

func (settings *DisplaySettings) zoomX(scale float64) {
	// FIXME: crashing when shifted
	bars := settings.bars()
	bars = (int)((float64)(bars) / scale)
	// bars must be bigger then 3 and smaller then max diff
	bars = (int)(math.Max((float64)(bars), 3))
	bars = (int)(math.Min((float64)(bars), (float64)(settings.maxTo - settings.minFrom)))
	// apply the scale
	settings.to = settings.from + bars
}

func (settings *DisplaySettings) shiftX(relativeStep float64) {
	bars := settings.bars()
	if relativeStep > 0 {
		settings.to += (int)((float64)(bars)/relativeStep)
		settings.to = (int)(math.Min((float64)(settings.to), (float64)(settings.maxTo)))
		settings.from = settings.to - bars
	} else {
		settings.from += (int)((float64)(bars)/relativeStep)
		settings.from = (int)(math.Max((float64)(settings.from), (float64)(settings.minFrom)))
		settings.to = settings.from + bars
	}
}

type Gui struct {
	width int32
	height int32
	window *sdl.Window
	surface *sdl.Surface
	settings DisplaySettings
}

func (gui *Gui) barWidth() int32 {
	return gui.width / (int32)(gui.settings.bars())
}

func (gui *Gui) clear() {
	everything := sdl.Rect{0, 0, gui.width, gui.height}
	gui.surface.FillRect(
		&everything,
		sdl.Color{255, 0, 0, 0}.Uint32(),
	)
}

func (gui *Gui) barRect(index int, value float64) sdl.Rect {
	var rect sdl.Rect
	barWidth := gui.barWidth()
	barHeight := (int32)((value / gui.settings.maxValue) * (float64)(gui.height))
	rect.X = (int32)(index-gui.settings.from) * barWidth
	rect.Y = gui.height - barHeight
	rect.W = barWidth
	rect.H = (int32)(barHeight)
	return rect
}

const peakHeight = 3
func (gui *Gui) drawPeak(peak Peak) {
	peakRect := gui.barRect(peak.index, peak.value)
	peakColor := sdl.Color{255, 0, 255, 0}.Uint32()
	peakRect.Y -= peakHeight
	peakRect.X -= gui.barWidth()
	peakRect.W = gui.barWidth()*3
	peakRect.H = peakHeight
	gui.surface.FillRect(&peakRect, peakColor)
}

func (gui *Gui) drawPeaks(data *AggregatedData) {
	for _, peak := range data.peaks {
		gui.drawPeak(peak)
	}
}

func (gui *Gui) drawBar(index int, value float64) {
	barColor := sdl.Color{255, 255, 0, 0}.Uint32()
	barRect := gui.barRect(index, value)
	gui.surface.FillRect(&barRect, barColor)
}

func (gui *Gui) drawBars(data *AggregatedData) {
	for i := gui.settings.from; i < gui.settings.to; i++ {
		gui.drawBar(i, data.values[i])
	}
}

func (gui *Gui) drawScales() {
	var scales = []int32{100, 10, 5, 2}
	bars := gui.settings.bars()
	var scale int32
	var scaleRect sdl.Rect
	scaleColor := sdl.Color{255, 255, 255, 0}.Uint32()
	for _, potentialScale := range scales {
		scale = potentialScale
		if (int32)(bars) / potentialScale > 3 {
			break
		}
	}
	scaleRect.Y = 0
	scaleRect.W = gui.width / (int32)(bars)
	scaleRect.H = gui.height
	for x := -((int32)(gui.settings.from) % scale); x <= (int32)(bars); x += scale {
		scaleRect.X = x * scaleRect.W
		gui.surface.FillRect(&scaleRect, scaleColor)
	}
}

func (gui *Gui) flip() {
	gui.window.UpdateSurface()
}

type Peak struct {
	index int
	value float64
}

type AggregatedData struct {
	values []float64
	peaks []Peak
}

func (data *AggregatedData) init(options Options) {
	data.values = make([]float64, options.samples/2)
	data.peaks = make([]Peak, 0, options.samples/2)
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

func (data *AggregatedData) updatePeaks(minPeakValue float64) {
	// delete peaks first
	data.peaks = make([]Peak, 0, cap(data.peaks))
	prev := math.Inf(-1)
	next := math.Inf(-1)
	maxIndex := len(data.values)-1
	for i, value := range data.values {
		if i >= maxIndex {
			next = math.Inf(-1)
		} else {
			next = data.values[i+1]
		}
		if i >= 1 && value >= minPeakValue && value > prev && value > next {
			data.peaks = append(data.peaks, Peak{i, value})
		}
		prev = value
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

func (data *AudioData) sumMagnitudeAt(index int) float64 {
	var sum float64 = 0
	for j := 0; j < data.size; j++ {
		sum += magnitude(data.values[j].Elems[index])
	}
	return (float64)(sum)
}

func (data *AudioData) avgMagnitudeAt(index int) float64 {
	return data.sumMagnitudeAt(index) / (float64)(data.size)
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
	flag.Float64Var(
		&options.minPeakValue, "min-peak-value", 0.5, "Minimal value to be considered as peak",
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
	running := true
	capturing := true
	currentData := new(AggregatedData)
	// start capturing data
	currentData.init(options)
	sdl.PauseAudioDevice(recordData.device, !capturing)
	for running {
		// process events
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
						gui.settings.zoomY(0.75)
					case sdl.K_DOWN:
						gui.settings.zoomY(1.25)
					case sdl.K_EQUALS:
						fallthrough
					case sdl.K_KP_PLUS:
						gui.settings.zoomX(2)
					case sdl.K_MINUS:
						fallthrough
					case sdl.K_KP_MINUS:
						gui.settings.zoomX(0.5)
					case sdl.K_RIGHT:
						gui.settings.shiftX(10)
					case sdl.K_LEFT:
						gui.settings.shiftX(-10)
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
		// calculate and display if capturing data
		if capturing {
			currentData.update(recordData)
			currentData.updatePeaks(options.minPeakValue)
		}
		// display when in debug mode
		if options.debug || options.tune {
			gui.clear()
			gui.drawScales()
			gui.drawPeaks(currentData)
			gui.drawBars(currentData)
			gui.flip()
		}
		if !options.tune {
			
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
	gui.settings.init(options.samples/2-1)
	recordData = new(AudioData)
	recordData.init(options)
	init_sdl(options, &gui)
	mainloop(options, &gui)
	sdl.Quit()
}
