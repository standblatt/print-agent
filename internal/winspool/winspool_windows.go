package winspool

import (
	"context"
	"errors"
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/standblatt/print-agent/internal/agent"
)

var (
	winspoolDLL = windows.NewLazySystemDLL("winspool.drv")
	gdi32       = windows.NewLazySystemDLL("gdi32.dll")

	procEnumPrinters       = winspoolDLL.NewProc("EnumPrintersW")
	procOpenPrinter        = winspoolDLL.NewProc("OpenPrinterW")
	procClosePrinter       = winspoolDLL.NewProc("ClosePrinter")
	procDocumentProperties = winspoolDLL.NewProc("DocumentPropertiesW")
	procDeviceCapabilities = winspoolDLL.NewProc("DeviceCapabilitiesW")

	procCreateDC          = gdi32.NewProc("CreateDCW")
	procDeleteDC          = gdi32.NewProc("DeleteDC")
	procGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")
	procStartDoc          = gdi32.NewProc("StartDocW")
	procEndDoc            = gdi32.NewProc("EndDoc")
	procAbortDoc          = gdi32.NewProc("AbortDoc")
	procStartPage         = gdi32.NewProc("StartPage")
	procEndPage           = gdi32.NewProc("EndPage")
	procSetStretchBltMode = gdi32.NewProc("SetStretchBltMode")
	procStretchDIBits     = gdi32.NewProc("StretchDIBits")
)

// Constants from wingdi.h and winspool.h.
const (
	printerEnumLocal       = 2
	printerEnumConnections = 4

	dmOutBuffer = 2
	dmInBuffer  = 8

	dmOrientation   = 0x1
	dmPaperSize     = 0x2
	dmDefaultSource = 0x200
	dmColor         = 0x800
	dmDuplex        = 0x1000

	orientPortrait  = 1
	orientLandscape = 2

	dcPapers      = 2
	dcBins        = 6
	dcDuplex      = 7
	dcBinNames    = 12
	dcPaperNames  = 16
	dcColorDevice = 32

	horzRes      = 8
	vertRes      = 10
	logPixelsX   = 88
	logPixelsY   = 90
	physicalOffX = 112
	physicalOffY = 113

	halftone = 4
	srcCopy  = 0x00CC0020

	// maxDPI caps the render resolution; GDI stretches to the printer's.
	maxDPI = 300
)

// devmode is DEVMODEW with the printer variant of its first union; the driver's private data follows it in the
// same buffer.
type devmode struct {
	DeviceName       [32]uint16
	SpecVersion      uint16
	DriverVersion    uint16
	Size             uint16
	DriverExtra      uint16
	Fields           uint32
	Orientation      int16
	PaperSize        int16
	PaperLength      int16
	PaperWidth       int16
	Scale            int16
	Copies           int16
	DefaultSource    int16
	PrintQuality     int16
	Color            int16
	Duplex           int16
	YResolution      int16
	TTOption         int16
	Collate          int16
	FormName         [32]uint16
	LogPixels        uint16
	BitsPerPel       uint32
	PelsWidth        uint32
	PelsHeight       uint32
	Nup              uint32
	DisplayFrequency uint32
	ICMMethod        uint32
	ICMIntent        uint32
	MediaType        uint32
	DitherType       uint32
	Reserved1        uint32
	Reserved2        uint32
	PanningWidth     uint32
	PanningHeight    uint32
}

// printerInfo2 is PRINTER_INFO_2W.
type printerInfo2 struct {
	ServerName, PrinterName, ShareName, PortName, DriverName, Comment, Location *uint16
	DevMode                                                                     uintptr
	SepFile, PrintProcessor, Datatype, Parameters                               *uint16
	SecurityDescriptor                                                          uintptr
	Attributes, Priority, DefaultPriority, StartTime, UntilTime, Status, Jobs   uint32
	AveragePPM                                                                  uint32
}

// docInfo is DOCINFOW.
type docInfo struct {
	Size     int32
	DocName  *uint16
	Output   *uint16
	Datatype *uint16
	Type     uint32
}

// bitmapInfoHeader is BITMAPINFOHEADER; for 24 bit pixels it is the whole BITMAPINFO.
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// Winspool prints through the Windows spooler.
type Winspool struct {
	Renderer *Renderer
}

// printer is a local printer or printer connection with its port, which DeviceCapabilities needs.
type printer struct{ Name, Port string }

// printers lists the local printers and printer connections.
func printers() ([]printer, error) {
	var needed, returned uint32
	procEnumPrinters.Call(printerEnumLocal|printerEnumConnections, 0, 2, 0, 0,
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return nil, nil
	}
	buf := make([]byte, needed)
	r, _, err := procEnumPrinters.Call(printerEnumLocal|printerEnumConnections, 0, 2, uintptr(unsafe.Pointer(&buf[0])),
		uintptr(needed), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r == 0 {
		return nil, fmt.Errorf("EnumPrinters: %w", err)
	}
	var out []printer
	for _, p := range unsafe.Slice((*printerInfo2)(unsafe.Pointer(&buf[0])), returned) {
		out = append(out, printer{windows.UTF16PtrToString(p.PrinterName), windows.UTF16PtrToString(p.PortName)})
	}
	return out, nil
}

// find returns the printer named name.
func find(name string) (printer, error) {
	ps, err := printers()
	if err != nil {
		return printer{}, err
	}
	for _, p := range ps {
		if p.Name == name {
			return p, nil
		}
	}
	return printer{}, fmt.Errorf("printer %q not found", name)
}

// capability calls DeviceCapabilitiesW; with out nil it returns the number of entries.
func capability(p printer, capability int, out unsafe.Pointer) int {
	name, _ := windows.UTF16PtrFromString(p.Name)
	port, _ := windows.UTF16PtrFromString(p.Port)
	r, _, _ := procDeviceCapabilities.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(port)),
		uintptr(capability), uintptr(out), 0)
	return int(int32(r))
}

// entries reads the IDs of idCap and the names of nameCap, width WCHARs each.
func entries(p printer, idCap, nameCap, width int) []Entry {
	n := capability(p, idCap, nil)
	if n <= 0 {
		return nil
	}
	ids := make([]uint16, n)
	names := make([]uint16, n*width)
	if capability(p, idCap, unsafe.Pointer(&ids[0])) != n || capability(p, nameCap, unsafe.Pointer(&names[0])) != n {
		return nil
	}
	out := make([]Entry, n)
	for i := range out {
		out[i] = Entry{ID: int16(ids[i]), Name: windows.UTF16ToString(names[i*width : (i+1)*width])}
	}
	return out
}

// documentProperties returns the printer's default DEVMODE with the changes of edit, merged and validated by the
// driver; edit nil returns the defaults.
func documentProperties(name string, edit func(*devmode)) ([]byte, error) {
	n16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	var h windows.Handle
	if r, _, err := procOpenPrinter.Call(uintptr(unsafe.Pointer(n16)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return nil, fmt.Errorf("OpenPrinter: %w", err)
	}
	defer procClosePrinter.Call(uintptr(h))
	size, _, _ := procDocumentProperties.Call(0, uintptr(h), uintptr(unsafe.Pointer(n16)), 0, 0, 0)
	if int32(size) <= 0 {
		return nil, errors.New("DocumentProperties: no DEVMODE size")
	}
	buf := make([]byte, size)
	if r, _, err := procDocumentProperties.Call(0, uintptr(h), uintptr(unsafe.Pointer(n16)), uintptr(unsafe.Pointer(&buf[0])), 0, dmOutBuffer); int32(r) < 0 {
		return nil, fmt.Errorf("DocumentProperties: %w", err)
	}
	if edit == nil {
		return buf, nil
	}
	edit((*devmode)(unsafe.Pointer(&buf[0])))
	if r, _, err := procDocumentProperties.Call(0, uintptr(h), uintptr(unsafe.Pointer(n16)), uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&buf[0])), dmInBuffer|dmOutBuffer); int32(r) < 0 {
		return nil, fmt.Errorf("DocumentProperties: %w", err)
	}
	return buf, nil
}

// caps reads trays, papers, duplex and colour capability and the defaults of a printer; what the driver does not
// answer stays empty.
func caps(p printer) Caps {
	c := Caps{
		Bins:   entries(p, dcBins, dcBinNames, 24),
		Papers: entries(p, dcPapers, dcPaperNames, 64),
		Duplex: capability(p, dcDuplex, nil) == 1,
		Color:  capability(p, dcColorDevice, nil) == 1,
	}
	if buf, err := documentProperties(p.Name, nil); err == nil {
		dm := (*devmode)(unsafe.Pointer(&buf[0]))
		c.Default = Settings{Bin: dm.DefaultSource, Paper: dm.PaperSize, Duplex: dm.Duplex, Color: dm.Color}
	}
	return c
}

// Printers lists the printers of the spooler with their options.
func (w *Winspool) Printers(context.Context) ([]agent.Printer, error) {
	ps, err := printers()
	if err != nil {
		return nil, err
	}
	out := []agent.Printer{}
	for _, p := range ps {
		out = append(out, agent.Printer{Name: p.Name, Options: Options(caps(p))})
	}
	return out, nil
}

// Print prints a job with its options on the printer it names.
func (w *Winspool) Print(ctx context.Context, j *agent.Job) error {
	p, err := find(j.Printer)
	if err != nil {
		return err
	}
	s, err := Translate(j.Options, caps(p))
	if err != nil {
		return err
	}
	return w.print(ctx, p.Name, j.Title, j.PDF, s, "")
}

// print renders the PDF page by page onto a printer DC set up with s; a non-empty output makes the spooler write
// the job into that file instead of to the printer's port.
func (w *Winspool) print(ctx context.Context, name, title string, pdf []byte, s Settings, output string) error {
	doc, err := w.Renderer.Open(pdf)
	if err != nil {
		return err
	}
	defer doc.Close()
	sizes, err := doc.Pages()
	if err != nil {
		return err
	}
	if len(sizes) == 0 {
		return errors.New("the PDF has no pages")
	}
	dm, err := documentProperties(name, func(dm *devmode) {
		if s.Bin != 0 {
			dm.DefaultSource, dm.Fields = s.Bin, dm.Fields|dmDefaultSource
		}
		if s.Paper != 0 {
			dm.PaperSize, dm.Fields = s.Paper, dm.Fields|dmPaperSize
		}
		if s.Duplex != 0 {
			dm.Duplex, dm.Fields = s.Duplex, dm.Fields|dmDuplex
		}
		if s.Color != 0 {
			dm.Color, dm.Fields = s.Color, dm.Fields|dmColor
		}
		dm.Orientation, dm.Fields = orientPortrait, dm.Fields|dmOrientation
		if sizes[0][0] > sizes[0][1] {
			dm.Orientation = orientLandscape
		}
	})
	if err != nil {
		return err
	}

	n16, _ := windows.UTF16PtrFromString(name)
	spool, _ := windows.UTF16PtrFromString("WINSPOOL")
	dc, _, err := procCreateDC.Call(uintptr(unsafe.Pointer(spool)), uintptr(unsafe.Pointer(n16)), 0, uintptr(unsafe.Pointer(&dm[0])))
	if dc == 0 {
		return fmt.Errorf("CreateDC: %w", err)
	}
	defer procDeleteDC.Call(dc)
	d := Device{
		DPIX: deviceCaps(dc, logPixelsX), DPIY: deviceCaps(dc, logPixelsY),
		OffX: deviceCaps(dc, physicalOffX), OffY: deviceCaps(dc, physicalOffY),
		HorzRes: deviceCaps(dc, horzRes), VertRes: deviceCaps(dc, vertRes),
	}
	if title == "" {
		title = "Standblatt"
	}
	di := docInfo{DocName: windows.StringToUTF16Ptr(title)}
	di.Size = int32(unsafe.Sizeof(di))
	if output != "" {
		di.Output = windows.StringToUTF16Ptr(output)
	}
	if r, _, err := procStartDoc.Call(dc, uintptr(unsafe.Pointer(&di))); int32(r) <= 0 {
		return fmt.Errorf("StartDoc: %w", err)
	}
	for i, size := range sizes {
		err := ctx.Err()
		if err == nil {
			err = printPage(dc, doc, i, Place(size[0], size[1], d, s.Scale), d)
		}
		if err != nil {
			procAbortDoc.Call(dc)
			return fmt.Errorf("page %d: %w", i+1, err)
		}
	}
	if r, _, err := procEndDoc.Call(dc); int32(r) <= 0 {
		return fmt.Errorf("EndDoc: %w", err)
	}
	return nil
}

// deviceCaps reads one GetDeviceCaps value.
func deviceCaps(dc uintptr, index int) int {
	r, _, _ := procGetDeviceCaps.Call(dc, uintptr(index))
	return int(int32(r))
}

// printPage renders page i at up to maxDPI and stretches it into t on a new printer page.
func printPage(dc uintptr, doc *Document, i int, t Rect, d Device) error {
	f := min(1, maxDPI/float64(d.DPIX))
	img, done, err := doc.Render(i, round(float64(t.W)*f), round(float64(t.H)*f))
	if err != nil {
		return err
	}
	defer done()
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	bits := bgr(img)
	if r, _, err := procStartPage.Call(dc); int32(r) <= 0 {
		return fmt.Errorf("StartPage: %w", err)
	}
	procSetStretchBltMode.Call(dc, halftone)
	bmi := bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 24}
	bmi.Size = uint32(unsafe.Sizeof(bmi))
	// Negative coordinates (noscale, the page corner left of and above the printable area) pass through as the
	// low 32 bits, which is what the int parameters read.
	lines, _, err := procStretchDIBits.Call(dc, uintptr(t.X), uintptr(t.Y), uintptr(t.W), uintptr(t.H), 0, 0,
		uintptr(w), uintptr(h), uintptr(unsafe.Pointer(&bits[0])), uintptr(unsafe.Pointer(&bmi)), 0, srcCopy)
	if int32(lines) <= 0 {
		return fmt.Errorf("StretchDIBits: %w", err)
	}
	if r, _, err := procEndPage.Call(dc); int32(r) <= 0 {
		return fmt.Errorf("EndPage: %w", err)
	}
	return nil
}

// bgr converts an RGBA image to top-down 24 bit BGR rows padded to 4 bytes, as a DIB needs them.
func bgr(img *image.RGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	stride := (w*3 + 3) &^ 3
	out := make([]byte, stride*h)
	for y := range h {
		src := img.Pix[y*img.Stride:]
		dst := out[y*stride:]
		for x := range w {
			dst[x*3], dst[x*3+1], dst[x*3+2] = src[x*4+2], src[x*4+1], src[x*4]
		}
	}
	return out
}
