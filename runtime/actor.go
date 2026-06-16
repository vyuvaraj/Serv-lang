package runtime

type ActorMessage struct {
	Payload interface{}
	Reply   chan interface{}
}

// ActorRef represents a reference to a running actor.
type ActorRef struct {
	Mailbox chan ActorMessage
}

// ActorSend sends a message to an actor's mailbox asynchronously.
func ActorSend(actor interface{}, msg interface{}) interface{} {
	ref, ok := actor.(*ActorRef)
	if !ok || ref == nil || ref.Mailbox == nil {
		return nil
	}
	ref.Mailbox <- ActorMessage{Payload: msg}
	return nil
}

// ActorAsk sends a message to an actor's mailbox synchronously and blocks waiting for a reply.
func ActorAsk(actor interface{}, msg interface{}) interface{} {
	ref, ok := actor.(*ActorRef)
	if !ok || ref == nil || ref.Mailbox == nil {
		return nil
	}
	replyChan := make(chan interface{}, 1)
	ref.Mailbox <- ActorMessage{Payload: msg, Reply: replyChan}
	return <-replyChan
}
