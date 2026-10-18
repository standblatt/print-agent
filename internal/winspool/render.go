package winspool

import (
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// Renderer renders PDF pages with PDFium in WebAssembly. Compiling the module takes up to several seconds, so the
// code wazero compiles is kept in CacheDir and the app starts the renderer in the background.
type Renderer struct {
	CacheDir string // empty compiles on every start

	once sync.Once
	pool pdfium.Pool
	inst pdfium.Pdfium
	err  error
}

// Start starts PDFium once; later calls return the result of the first.
func (r *Renderer) Start() error {
	r.once.Do(func() {
		// The bundled module needs exception handling, see webassembly.Config.RuntimeConfig.
		rc := wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)
		if r.CacheDir != "" {
			cache, err := wazero.NewCompilationCacheWithDir(r.CacheDir)
			if err != nil {
				r.err = fmt.Errorf("pdfium cache: %w", err)
				return
			}
			rc = rc.WithCompilationCache(cache)
		}
		// An empty FSConfig keeps PDFium away from the file system; documents come as bytes.
		r.pool, r.err = webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1, FSConfig: wazero.NewFSConfig(), RuntimeConfig: rc})
		if r.err != nil {
			return
		}
		r.inst, r.err = r.pool.GetInstance(30 * time.Second)
	})
	return r.err
}

// Document is an open PDF.
type Document struct {
	r   *Renderer
	doc references.FPDF_DOCUMENT
}

// Open starts the renderer if needed and opens a PDF.
func (r *Renderer) Open(pdf []byte) (*Document, error) {
	if err := r.Start(); err != nil {
		return nil, err
	}
	d, err := r.inst.OpenDocument(&requests.OpenDocument{File: &pdf})
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	return &Document{r: r, doc: d.Document}, nil
}

// Close releases the document.
func (d *Document) Close() {
	d.r.inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: d.doc})
}

// page references page i.
func (d *Document) page(i int) requests.Page {
	return requests.Page{ByIndex: &requests.PageByIndex{Document: d.doc, Index: i}}
}

// Pages returns the size of every page in points.
func (d *Document) Pages() ([][2]float64, error) {
	n, err := d.r.inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: d.doc})
	if err != nil {
		return nil, err
	}
	sizes := make([][2]float64, n.PageCount)
	for i := range sizes {
		s, err := d.r.inst.GetPageSize(&requests.GetPageSize{Page: d.page(i)})
		if err != nil {
			return nil, err
		}
		sizes[i] = [2]float64{s.Width, s.Height}
	}
	return sizes, nil
}

// Render renders page i into at most w×h pixels on white; the image is valid until done is called.
func (d *Document) Render(i, w, h int) (img *image.RGBA, done func(), err error) {
	res, err := d.r.inst.RenderPageInPixels(&requests.RenderPageInPixels{Page: d.page(i), Width: w, Height: h})
	if err != nil {
		return nil, nil, err
	}
	return res.Result.RenderedImage.(*image.RGBA), res.Cleanup, nil
}
