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
	"github.com/jvlmdr/go-fftw/fftw"
)

type audioData struct {
	mux sync.Mutex
	values[] *fftw.Array
	size int
	counter int
}

/*
type freqRange struct {
	min int,
	max int,
    treshold float64,
}

ranges := map[string]freqRange{
	"c" : {46, 52, 20},
	"d" : {0, 0, 0},
	"e" : {0, 0, 0},
	"f" : {0, 0, 0},
	"g" : {0, 0, 0},
	"a" : {0, 0, 0},
	"b" : {0, 0, 0},
	"c" : {0, 0, 0},
}
*/

var recordData audioData

func drawBar(surface *sdl.Surface, width int32, height int32, bars int32, index int32, maxValue float32, value float32) {
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
	rect := sdl.Rect{x, y, w, (int32)(h)}
	//rect := sdl.Rect{offset, height - (int32)(h), 2, 2}
	//fmt.Printf("Rect: %v\n", rect)
	surface.FillRect(&rect, sdl.Color{255, 255, 0, 0}.Uint32())
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

func drawBars(surface *sdl.Surface, width int32, height int32) {
	// locks
	recordData.mux.Lock()
	defer recordData.mux.Unlock()
	// continue with code
	var maximum float64
	var magnitude float32
	from := 0
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
		magnitude = displayMinMagnitude(i)
		maximum = math.Max(maximum, (float64)(magnitude))
		drawBar(
			surface, width, height,
			(int32)(bars), (int32)(i - from),
			(float32)(100), magnitude,
		)
	}
	fmt.Printf("Detected maximum: %v\n", maximum)
}

func mainloop(mainWindow *sdl.Window, recordDevice sdl.AudioDeviceID) {
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
			drawBars(surface, w, h)
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
	recordData.size = 3
	recordData.counter = 0
	recordData.values = make([]*fftw.Array, recordData.size)
	for i := 0; i < recordData.size; i++ {
		recordData.values[i] = fftw.NewArray(2048)
	}
	error = sdl.Init(sdl.INIT_VIDEO | sdl.INIT_AUDIO)
	printAudioRecordDevices()
	fmt.Printf("error: %v\n", error)
	mainWindow, error = sdl.CreateWindow("", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, 2048, 1080, 0)
	fmt.Printf("error: %v\n", error)
	//audioDevice, error = openRecordDevice(0)
	audioDevice, error = openRecordDevice(0)
	fmt.Printf("error: %v\n", error)
	mainloop(mainWindow, audioDevice)
	sdl.Quit()
}
