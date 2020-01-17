package main

/*
typedef unsigned char Uint8;
void recordCallback(void* userdata, Uint8* stream, int len);
*/
import "C"

import (
	"fmt"
	"unsafe"
	"sync"
	"math"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
	"github.com/jvlmdr/go-fftw/fftw"
)

type audioData struct {
	mux sync.Mutex
	values[] *fftw.Array
	size int
	counter int
}

type Peak struct {
    index int
    magnitude float64
}

type Stats struct {
	maximum float64
	peaks []Peak
}

var recordData audioData
var peaks map[string][]Peak
var font *ttf.Font

func drawBar(surface *sdl.Surface, width int32, height int32, bars int32, index int32, maxValue float32, value float32, peak bool) {
	var scales = []int32{100, 10, 5, 2}
	var scale int32
	for _, potentialScale := range scales {
		scale = potentialScale
		if bars / potentialScale > 5 {
			break
		}
	}

	w := (int32)(width / bars)
	if w <= 0 {
		w = (int32)(1)
	}
	x := index*w
	h := (value / maxValue) * (float32)(height)
	y := height - (int32)(h)
	if h < 0 {
		h = -h
		y = height
	}
	if index % scale == 0 {
		ref_rect := sdl.Rect{x, 0, w, height}
		surface.FillRect(&ref_rect, sdl.Color{255, 0, 255, 0}.Uint32())
	}
	rect := sdl.Rect{x, y, w, (int32)(h)}
	//rect := sdl.Rect{offset, height - (int32)(h), 2, 2}
	//fmt.Printf("Rect: %v\n", rect)
	surface.FillRect(&rect, sdl.Color{255, 255, 0, 0}.Uint32())
	if peak {
		rect.X = rect.X - 5
		rect.W = 10
		rect.H = 1
		surface.FillRect(&rect, sdl.Color{255, 255, 255, 0}.Uint32())
	}
}

func magnitude(item complex128) float64 {
	return math.Sqrt((float64)(real(item)*real(item) + imag(item)*imag(item)))
}

func displayMaxMagnitude(i int) float32 {
	var item float64 = 0
	for j := 0; j < recordData.size; j++ {
		item = math.Max(item, magnitude(recordData.values[j].Elems[i]))
	}
	return (float32)(item)
}

func displayMinMagnitude(i int) float32 {
	var item float64 = math.Inf(1)
	for j := 0; j < recordData.size; j++ {
		item = math.Min(item, magnitude(recordData.values[j].Elems[i]))
	}
	return (float32)(item)
}

func displaySumMagnitude(i int) float32 {
	var sum float64 = 0
	for j := 0; j < recordData.size; j++ {
		sum += magnitude(recordData.values[j].Elems[i])
	}
	return (float32)(sum)
}

func displayAvgMagnitude(i int) float32 {
	return displaySumMagnitude(i) / (float32)(recordData.size)
}

func freqToIndex(freq int, samples int, rate int) int {
	f := (float64)(freq)
	s := (float64)(samples)
	r := (float64)(rate)
	return (int)(math.Floor(f*s/r))
}

func displayData() []float32 {
	// locks
	recordData.mux.Lock()
	defer recordData.mux.Unlock()
	data := make([]float32, 2048/2)
	for i, _ := range(data) {
		data[i] = displayAvgMagnitude(i)
	}
	return data
}

func isPeak(data []float32, i int) bool {
	return data[i] > 2 && data[i] >= data[i-1] && data[i] >= data[i+1]
}

func drawBars(surface *sdl.Surface, width int32, height int32) float64 {
	// continue with code
	var maximum float64
	var magnitude float32
	data := displayData()
	from := 1
	to := 2048/2 - 1
	//from = freqToIndex(350, 2048, 44100)
	//to = freqToIndex(450, 2048, 44100)
	//fmt.Printf("From: %v to: %v\n", from, to)
	//from = 252
	//to = 255
	//from = 46
	//to = 52

	bars := to - from
	maximum = 0
	for i := from; i < to; i++ {
		magnitude = data[i]
		maximum = math.Max(maximum, (float64)(magnitude))
		drawBar(
			surface, width, height,
			(int32)(bars), (int32)(i - from),
			(float32)(100), magnitude,
			isPeak(data, i),
		)
	}
	return maximum
}

func printStats(surface *sdl.Surface, stats Stats) {
	text := fmt.Sprintf("Detected maximum: %v", stats.maximum)
	rendered, _ := font.RenderUTF8Solid(text, sdl.Color{255, 255, 0, 0})
	src := sdl.Rect{0, 0, rendered.W, rendered.H}
	dst := sdl.Rect{0, 0, surface.W, surface.H}
	rendered.Blit(&src, surface, &dst)
}

func mainloop(mainWindow *sdl.Window, recordDevice sdl.AudioDeviceID, stats Stats) {
	w, h := mainWindow.GetSize()
	everything := sdl.Rect{0, 0, w, h}
	surface, _ := mainWindow.GetSurface()
	draw := true
	running := true
	sdl.PauseAudioDevice(recordDevice, false)
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
						draw = !draw
					}
					break
				}
				break
			}
		}
		if (draw) {
			// clear
			surface.FillRect(
				&everything,
				sdl.Color{255, 0, 0, 0}.Uint32(),
			)
			// draw bar
			stats.maximum = drawBars(surface, w, h)
			printStats(surface, stats)
			// update window
			mainWindow.UpdateSurface()
			//fmt.Printf("%v iteration\n", i)
		}
		sdl.Delay(10)
	}
	sdl.PauseAudioDevice(recordDevice, true)
	fmt.Printf("END LOOP\n")
}

func audioRecordDevices() []string {
	devices := sdl.GetNumAudioDevices(true)
	var retval []string = make([]string, devices)
	for i := 0; i < devices; i++ {
		retval[i] = sdl.GetAudioDeviceName(i, true)
	}
	return retval
}

func printAudioRecordDevices() {
	for key, value := range audioRecordDevices() {
		fmt.Printf("Audio device #%v: %s\n", key, value)
	}
}

//export recordCallback
func recordCallback(userdata unsafe.Pointer, stream *C.Uint8, length C.int) {
	// locks
	recordData.mux.Lock()
	defer recordData.mux.Unlock()
	// continue with code
	index := recordData.counter % recordData.size
	dataSlice := (*[1<<30]float32)(unsafe.Pointer(stream))[:length/4:length/4]
	for i := 0; i < (int)(length/4); i++ {
		recordData.values[index].Elems[i] = (complex128)(complex(dataSlice[i], 0))
	}
	//fmt.Printf("Data: %v\n", len(dataSlice))
	fftw.FFTTo(recordData.values[index], recordData.values[index])
	recordData.counter++
	/*
	for i := len(recordData.values.Elems)-1; i >= 0; i--  {
		recordData.values.Elems[i] = complex(math.Log(real(recordData.values.Elems[i])), 0)
	}
	/**/
	//fmt.Printf("Data: %v\n", recordData.values.Elems)
	//fmt.Printf("Test\n")
}

func openRecordDevice(deviceId int) (sdl.AudioDeviceID, error) {
	var want, have sdl.AudioSpec
	want.Freq = 44100
	want.Format = sdl.AUDIO_F32SYS
	want.Channels = 1
	want.Samples = 2048
	want.Callback = sdl.AudioCallback(C.recordCallback)
	want.UserData = nil
	return sdl.OpenAudioDevice("", true, &want, &have, 0)
}

func main() {
	var error error
	var mainWindow *sdl.Window
	var audioDevice sdl.AudioDeviceID
	var stats Stats
	recordData.size = 3
	recordData.counter = 0
	recordData.values = make([]*fftw.Array, recordData.size)
	peaks = map[string][]Peak{
		"c" : make([]Peak, 0, 5),
		"d" : make([]Peak, 0, 5),
		"e" : make([]Peak, 0, 5),
		"f" : make([]Peak, 0, 5),
		"g" : make([]Peak, 0, 5),
		"a" : make([]Peak, 0, 5),
		"b" : make([]Peak, 0, 5),
	}

	for i := 0; i < recordData.size; i++ {
		recordData.values[i] = fftw.NewArray(2048)
	}
	error = sdl.Init(sdl.INIT_VIDEO | sdl.INIT_AUDIO)
	fmt.Printf("error: %v\n", error)
	if ! ttf.WasInit() {
		error = ttf.Init()
	}
	fmt.Printf("error: %v\n", error)
	printAudioRecordDevices()
	mainWindow, error = sdl.CreateWindow("", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, 2048, 1080, 0)
	fmt.Printf("error: %v\n", error)
	//audioDevice, error = openRecordDevice(0)
	audioDevice, error = openRecordDevice(0)
	fmt.Printf("error: %v\n", error)
	font, error = ttf.OpenFont("Sans.ttf", 12)
	fmt.Printf("error: %v\n", error)
	mainloop(mainWindow, audioDevice, stats)
	sdl.Quit()
}
