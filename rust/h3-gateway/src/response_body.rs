//! Bounded, backpressured transfer from the inherited H2 connection to H3.
use std::collections::VecDeque;
use std::time::Duration;

use anyhow::{Context, Result, anyhow};
use bytes::{Bytes, BytesMut};
use futures::SinkExt;
use http_body_util::BodyExt;
use hyper::body::Body;
use tokio::time::{Instant, timeout_at};
use tokio_quiche::http3::driver::{OutboundFrame, OutboundFrameSender};

use super::{BRIDGE_BULK_BODY_COALESCE_WAIT, BRIDGE_MAX_FRAME_SIZE};

// Compression sniffing must not withhold headers while a dynamic API response
// slowly produces its first 256 bytes. This is a total lookahead budget.
pub(super) async fn buffer_response_prefix<B>(
    body: &mut B,
    target: usize,
    budget: Duration,
) -> Result<(VecDeque<Bytes>, bool)>
where
    B: Body<Data = Bytes> + Unpin,
    B::Error: std::error::Error + Send + Sync + 'static,
{
    let deadline = Instant::now() + budget;
    let mut buffered = VecDeque::new();
    let mut size = 0;
    while size < target {
        if Instant::now() >= deadline {
            return Ok((buffered, false));
        }
        match timeout_at(deadline, body.frame()).await {
            Ok(Some(frame)) => {
                if let Ok(data) = frame?.into_data()
                    && !data.is_empty()
                {
                    size += data.len();
                    buffered.push_back(data);
                }
            }
            Ok(None) => return Ok((buffered, true)),
            Err(_) => return Ok((buffered, false)),
        }
    }
    Ok((buffered, false))
}

async fn flush_pending_body(pending: &mut BytesMut, send: &mut OutboundFrameSender) -> Result<()> {
    if !pending.is_empty() {
        // Allocate only when a later partial frame actually needs buffering.
        let bytes = std::mem::take(pending).freeze();
        send.send(OutboundFrame::Body(bytes, false)).await?;
    }
    Ok(())
}

async fn send_body_data(mut data: Bytes, send: &mut OutboundFrameSender, cap: usize) -> Result<()> {
    while !data.is_empty() {
        let take = data.len().min(cap);
        send.send(OutboundFrame::Body(data.split_to(take), false))
            .await?;
    }
    Ok(())
}

async fn append_coalesced_body(
    pending: &mut BytesMut,
    mut data: Bytes,
    send: &mut OutboundFrameSender,
    cap: usize,
) -> Result<()> {
    while !data.is_empty() {
        if pending.is_empty() && data.len() >= cap {
            // Large H2 DATA frames already have the desired shape. Bytes
            // slices retain ownership of the received buffer without copying.
            send.send(OutboundFrame::Body(data.split_to(cap), false))
                .await?;
            continue;
        }
        let take = data.len().min(cap - pending.len());
        pending.extend_from_slice(&data.split_to(take));
        if pending.len() == cap {
            flush_pending_body(pending, send).await?;
        }
    }
    Ok(())
}

pub(super) async fn forward_raw_body<B>(
    body: &mut B,
    mut prefix: VecDeque<Bytes>,
    send: &mut OutboundFrameSender,
    idle_timeout: Duration,
    coalesce_wait: Duration,
) -> Result<()>
where
    B: Body<Data = Bytes> + Unpin,
    B::Error: std::error::Error + Send + Sync + 'static,
{
    let cap = BRIDGE_MAX_FRAME_SIZE as usize;
    let mut first = prefix.is_empty();
    while let Some(data) = prefix.pop_front() {
        send_body_data(data, send, cap).await?;
    }
    let mut pending = BytesMut::new();
    let mut pending_deadline: Option<Instant> = None;
    loop {
        if pending_deadline.is_some_and(|deadline| Instant::now() >= deadline) {
            flush_pending_body(&mut pending, send).await?;
            pending_deadline = None;
        }
        let deadline = pending_deadline.unwrap_or_else(|| Instant::now() + idle_timeout);
        match timeout_at(deadline, body.frame()).await {
            Ok(Some(Ok(frame))) => {
                if let Ok(data) = frame.into_data()
                    && !data.is_empty()
                {
                    if first && coalesce_wait < BRIDGE_BULK_BODY_COALESCE_WAIT {
                        send_body_data(data, send, cap).await?;
                    } else {
                        append_coalesced_body(&mut pending, data, send, cap).await?;
                    }
                    first = false;
                    if pending.is_empty() {
                        pending_deadline = None;
                    } else {
                        // Do not restart the timer for every small frame: a
                        // steady trickle must not wait until 64 KiB accumulates.
                        pending_deadline.get_or_insert_with(|| {
                            Instant::now() + coalesce_wait.min(idle_timeout)
                        });
                    }
                }
            }
            Ok(Some(Err(error))) => {
                let _ = flush_pending_body(&mut pending, send).await;
                return Err(error).context("failed to read inherited HTTP/2 response body");
            }
            Ok(None) => {
                flush_pending_body(&mut pending, send).await?;
                return Ok(());
            }
            Err(_) if !pending.is_empty() => {
                flush_pending_body(&mut pending, send).await?;
                pending_deadline = None;
            }
            Err(_) => return Err(anyhow!("response body idle timeout")),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use futures::{StreamExt, stream};
    use http_body_util::StreamBody;
    use hyper::body::Frame;
    use std::convert::Infallible;
    use tokio_util::sync::PollSender;

    fn runtime() -> tokio::runtime::Runtime {
        tokio::runtime::Builder::new_current_thread()
            .enable_time()
            .build()
            .unwrap()
    }

    #[test]
    fn full_frames_reuse_received_storage() {
        runtime().block_on(async {
            let data = Bytes::from(vec![7; 128]);
            let ptr = data.as_ptr();
            let (tx, mut rx) = tokio::sync::mpsc::channel(8);
            let mut send = PollSender::new(tx);
            let mut pending = BytesMut::new();
            append_coalesced_body(&mut pending, data, &mut send, 64)
                .await
                .unwrap();
            let OutboundFrame::Body(first, false) = rx.recv().await.unwrap() else {
                panic!("missing body")
            };
            assert_eq!(first.as_ptr(), ptr);
            assert_eq!(first.len(), 64);
            let OutboundFrame::Body(second, false) = rx.recv().await.unwrap() else {
                panic!("missing body")
            };
            assert_eq!(second.as_ptr(), ptr.wrapping_add(64));
            assert_eq!(pending.capacity(), 0);
        });
    }

    #[test]
    fn mixed_frames_preserve_order_and_cap() {
        runtime().block_on(async {
            let (tx, mut rx) = tokio::sync::mpsc::channel(32);
            let mut send = PollSender::new(tx);
            let mut pending = BytesMut::new();
            for data in [b"ab".as_slice(), b"cdefghijkl", b"mn"] {
                append_coalesced_body(&mut pending, Bytes::copy_from_slice(data), &mut send, 4)
                    .await
                    .unwrap();
            }
            flush_pending_body(&mut pending, &mut send).await.unwrap();
            drop(send);
            let mut output = Vec::new();
            while let Some(frame) = rx.recv().await {
                let OutboundFrame::Body(data, false) = frame else {
                    panic!("unexpected frame")
                };
                assert!(data.len() <= 4);
                output.extend_from_slice(&data);
            }
            assert_eq!(output, b"abcdefghijklmn");
        });
    }

    #[test]
    fn compression_lookahead_does_not_wait_for_a_stalled_body() {
        runtime().block_on(async {
            let frames = stream::iter([Ok::<_, Infallible>(Frame::data(Bytes::from_static(b"{")))])
                .chain(stream::pending());
            let mut body = StreamBody::new(frames);
            let result = tokio::time::timeout(
                Duration::from_secs(1),
                buffer_response_prefix(&mut body, 256, Duration::from_millis(2)),
            )
            .await
            .unwrap()
            .unwrap();
            assert!(!result.1);
            assert_eq!(result.0.front().unwrap().as_ref(), b"{");
        });
    }

    #[test]
    fn first_live_frame_is_immediate_and_later_frames_are_coalesced() {
        runtime().block_on(async {
            let frames = stream::iter(
                [b"a", b"b", b"c"]
                    .map(|data| Ok::<_, Infallible>(Frame::data(Bytes::from_static(data)))),
            );
            let mut body = StreamBody::new(frames);
            let (tx, mut rx) = tokio::sync::mpsc::channel(8);
            let mut send = PollSender::new(tx);
            forward_raw_body(
                &mut body,
                VecDeque::new(),
                &mut send,
                Duration::from_secs(1),
                Duration::from_millis(2),
            )
            .await
            .unwrap();
            drop(send);
            let OutboundFrame::Body(first, false) = rx.recv().await.unwrap() else {
                panic!("missing body")
            };
            let OutboundFrame::Body(rest, false) = rx.recv().await.unwrap() else {
                panic!("missing body")
            };
            assert_eq!(first.as_ref(), b"a");
            assert_eq!(rest.as_ref(), b"bc");
            assert!(rx.recv().await.is_none());
        });
    }

    #[test]
    fn stalled_body_releases_transfer_with_an_error() {
        runtime().block_on(async {
            let mut body = StreamBody::new(stream::pending::<Result<Frame<Bytes>, Infallible>>());
            let (tx, _rx) = tokio::sync::mpsc::channel(1);
            let mut send = PollSender::new(tx);
            let result = tokio::time::timeout(
                Duration::from_secs(1),
                forward_raw_body(
                    &mut body,
                    VecDeque::new(),
                    &mut send,
                    Duration::from_millis(2),
                    Duration::from_millis(2),
                ),
            )
            .await
            .unwrap();
            assert!(result.unwrap_err().to_string().contains("idle timeout"));
        });
    }
}
