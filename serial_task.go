package nocan

import (
	"pannetrat.com/nocan/clog"
	"pannetrat.com/nocan/model"
	"pannetrat.com/nocan/serialcan"
	"time"
)

const (
	SERIAL_CAN_CTRL_RESET              = 1
	SERIAL_CAN_CTRL_GET_REGISTERS      = 2
	SERIAL_CAN_CTRL_SET_POWER          = 3
	SERIAL_CAN_CTRL_GET_POWER_STATUS   = 4
	SERIAL_CAN_CTRL_RESISTOR_CONFIGURE = 5
)

type SerialCanRequest struct {
	Status uint8
}

const (
	SERIAL_CAN_REQUEST_STATUS_UNKNOWN   = 0
	SERIAL_CAN_REQUEST_STATUS_SUBMITTED = 1
	SERIAL_CAN_REQUEST_STATUS_SUCCESS   = 2
	SERIAL_CAN_REQUEST_STATUS_FAILURE   = 2
)

type SerialTask struct {
	Serial        *serialcan.SerialCan
	Device        string
	InputBuffer   [128]*model.Message
	OutputBuffer  model.Message
	RequestStatus [6]SerialCanRequest
	PowerStatus   struct {
		PowerOn      bool
		PowerLevel   float32
		SenseOn      bool
		SenseLevel   float32
		UsbReference float32
	}
	RescueMode bool
}

func NewSerialTask(device string) *SerialTask {
	serial, err := serialcan.SerialCanOpen(device)
	if err != nil {
		clog.Error("Could not open %s: %s", device, err.Error())
		return nil
	}
	clog.Debug("Opened device %s", device)

	se := &SerialTask{Serial: serial, Device: device}

	return se
}

func (se *SerialTask) Close() {
	se.Serial.Close()
}

func (se *SerialTask) Rescue() bool {
	se.Close()
	for {
		serial, err := serialcan.SerialCanOpen(se.Device)
		if err == nil {
			se.Serial = serial
			clog.Info("Reopened device %s", se.Device)
			se.SendReset()
			return true
		} else {
			clog.Warning("Failed to reopen device %s: %s", se.Device, err.Error())
		}
		time.Sleep(10 * time.Second)
	}
}

/*
func (se *SerialTask) GetAttributes() interface{} {
	return &se.PowerStatus
}
*/

func (se *SerialTask) ProcessSend(task *model.TaskState) {
	for {
		var frame model.CanFrame

		//clog.Debug("Waiting for serial input")
		if se.Serial.Recv(&frame) == false {
			clog.Error("Failed to receive frame from %s", se.Device)
			if se.Rescue() {
				continue
			} else {
				break
			}
		}
		//clog.Debug("Got serial input")

		node := frame.CanId.GetNode()

		switch {
		case !frame.CanId.IsExtended(), frame.CanId.IsRemote():
			clog.Warning("Got malformed frame, discarding.")
		case frame.CanId.IsControl():
			switch frame.CanId.GetSysFunc() {
			case SERIAL_CAN_CTRL_RESET:
				if frame.CanId.GetSysParam() == 0 {
					se.RequestStatus[SERIAL_CAN_CTRL_RESET].Status = SERIAL_CAN_REQUEST_STATUS_SUCCESS
				} else {
					se.RequestStatus[SERIAL_CAN_CTRL_RESET].Status = SERIAL_CAN_REQUEST_STATUS_FAILURE
				}
			case SERIAL_CAN_CTRL_GET_POWER_STATUS:
				if frame.CanId.GetSysParam() == 0 {
					se.RequestStatus[SERIAL_CAN_CTRL_GET_POWER_STATUS].Status = SERIAL_CAN_REQUEST_STATUS_SUCCESS
					usbref := (uint16(frame.CanData[6]) << 8) | uint16(frame.CanData[7])
					powerlevel := (uint16(frame.CanData[1]) << 8) | uint16(frame.CanData[2])
					senselevel := (uint16(frame.CanData[4]) << 8) | uint16(frame.CanData[5])
					se.PowerStatus.PowerOn = frame.CanData[0] != 0
					se.PowerStatus.PowerLevel = float32(powerlevel) / float32(usbref) * 1.1 * 9.2
					se.PowerStatus.SenseOn = frame.CanData[3] != 0
					se.PowerStatus.SenseLevel = 100 * float32(senselevel) / 1023
					se.PowerStatus.UsbReference = 1023 * 1.1 / float32(usbref)
					clog.Info("Power stat estimates: power=%t power_level=%.1fV sense=%t sense_level=%.3f%% usb_power=%.1fV",
						se.PowerStatus.PowerOn, se.PowerStatus.PowerLevel,
						se.PowerStatus.SenseOn, se.PowerStatus.SenseLevel,
						se.PowerStatus.UsbReference)
				} else {
					se.RequestStatus[SERIAL_CAN_CTRL_GET_POWER_STATUS].Status = SERIAL_CAN_REQUEST_STATUS_FAILURE
				}
			}
		case frame.CanId.IsError():
			clog.Error("Recieved error frame on CAN controller")
		default:
			if frame.CanId.IsFirst() {
				if se.InputBuffer[node] != nil {
					clog.Warning("Got frame with inconsistent first bit indicator, discarding.")
					return
				}
				se.InputBuffer[node] = model.NewMessageFromFrame(&frame)
			} else {
				if se.InputBuffer[node] == nil {
					clog.Warning("Got first frame with missing first bit indicator, discarding.")
					return
				}
				se.InputBuffer[node].AppendData(frame.CanData[:frame.CanDlc])
			}
			if frame.CanId.IsLast() {
				task.SendMessage(se.InputBuffer[node])
				se.InputBuffer[node] = nil
			}
		}
	}
}

func (se *SerialTask) ProcessRecv(task *model.TaskState) {
	for {
		var frame model.CanFrame

		m, s := task.Recv()

		if m != nil {
			pos := 0
			for {
				frame.CanId = (m.Id & model.CANID_MASK_MESSAGE) | model.CANID_MASK_EXTENDED
				if pos == 0 {
					frame.CanId |= model.CANID_MASK_FIRST
				}
				if len(m.Data)-pos <= 8 {
					frame.CanId |= model.CANID_MASK_LAST
					frame.CanDlc = uint8(len(m.Data) - pos)
				} else {
					frame.CanDlc = 8
				}
				copy(frame.CanData[:], m.Data[pos:pos+int(frame.CanDlc)])
				if se.Serial.Send(&frame) == false {
					clog.Error("Failed to send frame to %s", se.Device)
					return
				}
				pos += int(frame.CanDlc)
				if pos >= len(m.Data) {
					break
				}
			}
		} else {
			if s.Value == model.SIGNAL_HEARTBEAT {
				se.SendGetPowerStatus()
			}
			// ignore other signals
		}
	}
}

func (se *SerialTask) Setup(_ *model.TaskState) {

}

func (se *SerialTask) Run(task *model.TaskState) {
	go se.ProcessRecv(task)
	se.SendReset()
	se.SendGetPowerStatus()
	se.ProcessSend(task)
}

/*
func MakeControlMessage(fn uint8, param uint8, data []byte) *Message {
    m := &Message{}
    m.Id = CanId(model.CANID_MASK_CONTROL).SetSysFunc(fn).SetSysParam(param)
    if data!=nil {
        m.Data = make([]byte,len(data))
        copy(m.Data[:],data)
    } else {
        m.Data = make([]byte,0)
    }
    return m
}
*/

func MakeControlFrame(fn uint8, param uint8, data []byte) *model.CanFrame {
	frame := &model.CanFrame{}
	frame.CanId = model.CanId(model.CANID_MASK_CONTROL).SetSysFunc(fn).SetSysParam(param)
	frame.CanDlc = uint8(len(data))
	if data != nil {
		copy(frame.CanData[:], data)
	}
	return frame
}

func (se *SerialTask) SendReset() {
	frame := MakeControlFrame(SERIAL_CAN_CTRL_RESET, 0, nil)
	se.RequestStatus[SERIAL_CAN_CTRL_RESET].Status = SERIAL_CAN_REQUEST_STATUS_SUBMITTED
	se.Serial.Send(frame)
}

func (se *SerialTask) SendGetPowerStatus() {
	frame := MakeControlFrame(SERIAL_CAN_CTRL_GET_POWER_STATUS, 0, nil)
	se.RequestStatus[SERIAL_CAN_CTRL_GET_POWER_STATUS].Status = SERIAL_CAN_REQUEST_STATUS_SUBMITTED
	se.Serial.Send(frame)
}