# Xylophone trigger

## Compiling
For compilation, following packages need to be installed:
- golang
- SDL2-devel
- SDL2_ttf-devel
- fftw-devel

The packages can be installed running command: `sudo dnf install golang SDL2-devel SDL2_ttf-devel fftw-devel`

Additionally, you need to combile go libraries used by analyzer:
```bash
go get github.com/veandco/go-sdl2/sdl
go get github.com/veandco/go-sdl2/ttf
go get github.com/jvlmdr/go-fftw/fftw
```
Finally, the analyzer can be compiled running command: `go build -o analyzer analyzer.go`

## Runtime dependencies
For running the analyzer, following packages need to be installed:
- SDL2
- SDL2_ttf
- fftw

The packages can be installed running command: `sudo dnf install SDL2 SDL2_ttf fftw`

## Configuration
Create config.txt containing information about tones. The easiest way to obtain information about the tones is by running the analyzer in debug mode: `./analyzer -debug`, emitting the tones and capturing peak information (together with the value of the peak). The capturing can be paused at any time by pressing space.

## Running the trigger
`./analyzer | ./trigger.py --keep-reading GBAD echo HIT`
