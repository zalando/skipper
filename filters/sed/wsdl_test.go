package sed

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

const testWSDL = `<?xml version='1.0' encoding='UTF-8'?>
        <wsdl:definitions xmlns:xsd="http://www.w3.org/2001/XMLSchema"
            xmlns:wsdl="http://schemas.xmlsoap.org/wsdl/"
            xmlns:tns="http://reading.webservice.price.example.org/"
            xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
            xmlns:ns2="http://schemas.xmlsoap.org/soap/http"
            xmlns:ns1="http://price.example.org/read/csvExport"
            name="CSVPriceReadWebServiceImplService"
            targetNamespace="http://reading.webservice.price.example.org/">
            <wsdl:import location="http://restsn08.example:37007/ws/csvPriceReadWebService?wsdl=CSVPriceReadWebService.wsdl"
                namespace="http://price.example.org/read/csvExport">
            </wsdl:import>
            <wsdl:binding name="CSVPriceReadWebServiceImplServiceSoapBinding"
                type="ns1:CSVPriceReadWebService">
                <soap:binding style="document"
                    transport="http://schemas.xmlsoap.org/soap/http"/>
                <wsdl:operation
                    name="getWholeCurrentPromotionalBlacklistZipFile">
                    <soap:operation soapAction="" style="document"/>
                        <wsdl:input
                            name="getWholeCurrentPromotionalBlacklistZipFile">
                            <soap:body use="literal"/>
                        </wsdl:input>
                        <wsdl:output
                            name="getWholeCurrentPromotionalBlacklistZipFileResponse">
                            <soap:body use="literal"/>
                        </wsdl:output>
                </wsdl:operation>
            </wsdl:binding>
            <wsdl:service name="CSVPriceReadWebServiceImplService">
                <wsdl:port 
                    binding="tns:CSVPriceReadWebServiceImplServiceSoapBinding"
                    name="CSVPriceReadWebServiceImplPort">
                    <soap:address
                        location="http://restsn08.example:37077/ws/csvPriceReadWebService"/>
                </wsdl:port>
            </wsdl:service>
        </wsdl:definitions>`

const patchedWSDL = `<?xml version='1.0' encoding='UTF-8'?>
        <wsdl:definitions xmlns:xsd="http://www.w3.org/2001/XMLSchema"
            xmlns:wsdl="http://schemas.xmlsoap.org/wsdl/"
            xmlns:tns="http://reading.webservice.price.example.org/"
            xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
            xmlns:ns2="http://schemas.xmlsoap.org/soap/http"
            xmlns:ns1="http://price.example.org/read/csvExport"
            name="CSVPriceReadWebServiceImplService"
            targetNamespace="http://reading.webservice.price.example.org/">
            <wsdl:import location="https://price-service.tm.example.org:37077/ws/csvPriceReadWebService?wsdl=CSVPriceReadWebService.wsdl"
                namespace="http://price.example.org/read/csvExport">
            </wsdl:import>
            <wsdl:binding name="CSVPriceReadWebServiceImplServiceSoapBinding"
                type="ns1:CSVPriceReadWebService">
                <soap:binding style="document"
                    transport="http://schemas.xmlsoap.org/soap/http"/>
                <wsdl:operation
                    name="getWholeCurrentPromotionalBlacklistZipFile">
                    <soap:operation soapAction="" style="document"/>
                        <wsdl:input
                            name="getWholeCurrentPromotionalBlacklistZipFile">
                            <soap:body use="literal"/>
                        </wsdl:input>
                        <wsdl:output
                            name="getWholeCurrentPromotionalBlacklistZipFileResponse">
                            <soap:body use="literal"/>
                        </wsdl:output>
                </wsdl:operation>
            </wsdl:binding>
            <wsdl:service name="CSVPriceReadWebServiceImplService">
                <wsdl:port 
                    binding="tns:CSVPriceReadWebServiceImplServiceSoapBinding"
                    name="CSVPriceReadWebServiceImplPort">
                    <soap:address
                        location="https://price-service.tm.example.org:37077/ws/csvPriceReadWebService"/>
                </wsdl:port>
            </wsdl:service>
        </wsdl:definitions>`

func TestWSDLExample(t *testing.T) {
	const (
		response = testWSDL
		expected = patchedWSDL
	)

	resp := &http.Response{Body: ioutil.NopCloser(strings.NewReader(response)),
		ContentLength: int64(len(response))}

	sp := New()
	conf := []interface{}{
		"location=\"https?://[^/]+/ws/",
		"location=\"https://price-service.tm.example.org:37077/ws/",
	}

	f, err := sp.CreateFilter(conf)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FResponse: resp}
	f.Response(ctx)

	body, err := ioutil.ReadAll(ctx.Response().Body)
	if err != nil {
		t.Error(err)
	}

	if l, hasContentLength := resp.Header["Content-Length"]; hasContentLength || resp.ContentLength != -1 {
		t.Error("Content-Length should not be set.", l, resp.ContentLength)
	}

	if string(body) != expected {
		t.Error("Expected \"" + expected + "\", got \"" + string(body) + "\"")
	}
}
