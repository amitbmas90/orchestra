/* marshal.go
 *
 * Common wire marshalling code.
*/

package orchestra;

import (
	"errors"
	"code.google.com/p/goprotobuf/proto"
)

var (
	ErrUnknownType = errors.New("Unknown Type in Encode request")
	ErrObjectTooLarge = errors.New("Encoded Object exceeds maximum encoding size")
)

/* ugh ugh ugh.  As much as I love protocol buffers, not having maps
 * as a native type is a PAIN IN THE ASS.
 *
 * Here's some common code to convert my K/V format in protocol
 * buffers to and from native Go structures.
*/
func MapFromProtoJobParameters(parray []*ProtoJobParameter) (mapparam map[string]string) {
	mapparam = make(map[string]string)

	for p := range parray {
		mapparam[*(parray[p].Key)] = *(parray[p].Value)
	}

	return mapparam
}

func ProtoJobParametersFromMap(mapparam map[string]string) (parray []*ProtoJobParameter) {
	parray = make([]*ProtoJobParameter, len(mapparam))
	i := 0
	for k,v := range mapparam {
		arg := new(ProtoJobParameter)
		arg.Key = proto.String(k)
		arg.Value = proto.String(v)
		parray[i] = arg
		i++
	}

	return parray
}



func (p *WirePkt) Decode() (obj interface{}, err error) {
	switch (p.Type) {
	case TypeNop:
		if (p.Length != 0) {
			/* throw error later... */
			return nil, ErrMalformedMessage;
		}
		return nil, nil
	case TypeIdentifyClient:
		ic := new(IdentifyClient)
		err := proto.Unmarshal(p.Payload[0:p.Length], ic)
		if err != nil {
			return nil, err
		}
		return ic, nil
	case TypeReadyForTask:
		if (p.Length != 0) {
			/* throw error later... */
			return nil, ErrMalformedMessage;
		}
		return nil, nil
	case TypeTaskRequest:
		tr := new(ProtoTaskRequest)
		err := proto.Unmarshal(p.Payload[0:p.Length], tr)
		if err != nil {
			return nil, err
		}
		return tr, nil
	case TypeTaskResponse:
		tr := new(ProtoTaskResponse)
		err := proto.Unmarshal(p.Payload[0:p.Length], tr)
		if err != nil {
			return nil, err
		}
		return tr, nil
	case TypeAcknowledgement:
		tr := new(ProtoAcknowledgement)
		err := proto.Unmarshal(p.Payload[0:p.Length], tr)
		if err != nil {
			return nil, err
		}
		return tr, nil
	}
	return nil, ErrUnknownMessage
}

func Encode(obj interface{}) (p *WirePkt, err error) {
	p = new(WirePkt)
	switch obj.(type) {
	case *IdentifyClient:
		p.Type = TypeIdentifyClient
	case *ProtoTaskRequest:
		p.Type = TypeTaskRequest
	case *ProtoTaskResponse:
		p.Type = TypeTaskResponse
	case *ProtoAcknowledgement:
		p.Type = TypeAcknowledgement
	default:
		Warn("Encoding unknown type!")
		return nil, ErrUnknownType
	}
	p.Payload, err = proto.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if len(p.Payload) >= 0x10000 {
		return nil, ErrObjectTooLarge
	}
	p.Length = uint16(len(p.Payload))

	return p, nil
}


func MakeNop() (p *WirePkt) {
	p = new(WirePkt)
	p.Length = 0
	p.Type = TypeNop
	p.Payload = nil

	return p
}

func MakeIdentifyClient(hostname string) (p *WirePkt) {
	s := new(IdentifyClient)
	s.Hostname = proto.String(hostname)

	p, _ = Encode(s)

	return p
}

func MakeReadyForTask() (p *WirePkt){
	p = new(WirePkt)
	p.Type = TypeReadyForTask
	p.Length = 0
	p.Payload = nil

	return p
}

/* We use the failure code for negative acknowledgements */
func MakeNack(id uint64) (p *WirePkt) {
	a := new(ProtoAcknowledgement)
	a.Id = proto.Uint64(id)
	a.Response = new(ProtoAcknowledgement_AckType)
	*(a.Response) = ProtoAcknowledgement_ACK_ERROR

	p, _ = Encode(a)

	return p
}

// Construct a positive ACK for transmission
func MakeAck(id uint64) (p *WirePkt) {
	a := new(ProtoAcknowledgement)
	a.Id = proto.Uint64(id)
	a.Response = new(ProtoAcknowledgement_AckType)
	*(a.Response) = ProtoAcknowledgement_ACK_OK

	p, _ = Encode(a)

	return p
}
