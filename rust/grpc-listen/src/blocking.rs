//! Admission control for finite CPU/filesystem RPC work. Waiting happens on
//! the async side, before occupying Tokio blocking threads or contending on
//! the search engine lock. A dropped queued RPC never starts its closure.
use std::sync::{Arc, OnceLock};
use tokio::sync::Semaphore;
use tonic::Status;

pub async fn run_blocking<F, T>(work: F) -> Result<T, Status>
where
    F: FnOnce() -> T + Send + 'static,
    T: Send + 'static,
{
    static PERMITS: OnceLock<Arc<Semaphore>> = OnceLock::new();
    let permits = PERMITS.get_or_init(|| {
        let threads = std::thread::available_parallelism().map_or(1, usize::from);
        Arc::new(Semaphore::new(threads.clamp(1, 32)))
    });
    run_with_permits(permits.clone(), work).await
}

async fn run_with_permits<F, T>(permits: Arc<Semaphore>, work: F) -> Result<T, Status>
where
    F: FnOnce() -> T + Send + 'static,
    T: Send + 'static,
{
    let permit = permits
        .acquire_owned()
        .await
        .map_err(|_| Status::unavailable("blocking worker shutting down"))?;
    tokio::task::spawn_blocking(move || {
        // Started blocking work cannot be aborted. Retain its slot even if
        // the RPC future is dropped, until the actual computation finishes.
        let _permit = permit;
        work()
    })
    .await
    .map_err(|err| Status::internal(format!("blocking worker join: {err}")))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicBool, Ordering};

    #[tokio::test]
    async fn canceled_waiter_never_starts() {
        let permits = Arc::new(Semaphore::new(1));
        let occupied = permits.clone().acquire_owned().await.unwrap();
        let called = Arc::new(AtomicBool::new(false));
        let marker = called.clone();
        let mut pending = Box::pin(run_with_permits(permits.clone(), move || {
            marker.store(true, Ordering::SeqCst)
        }));
        // Poll deterministically until the task is queued for admission.
        assert!(
            std::future::poll_fn(|cx| std::task::Poll::Ready(
                pending.as_mut().poll(cx).is_pending()
            ))
            .await
        );
        drop(pending);
        drop(occupied);
        assert_eq!(run_with_permits(permits, || 42).await.unwrap(), 42);
        assert!(!called.load(Ordering::SeqCst));
    }

    #[tokio::test]
    async fn canceled_running_call_keeps_its_permit() {
        let permits = Arc::new(Semaphore::new(1));
        let (started_tx, started_rx) = tokio::sync::oneshot::channel();
        let (finish_tx, finish_rx) = std::sync::mpsc::channel();
        let slots = permits.clone();
        let task = tokio::spawn(run_with_permits(slots, move || {
            started_tx.send(()).unwrap();
            finish_rx.recv().unwrap();
        }));
        started_rx.await.unwrap();
        task.abort();
        let _ = task.await;
        assert_eq!(permits.available_permits(), 0);
        finish_tx.send(()).unwrap();
        assert_eq!(run_with_permits(permits, || 42).await.unwrap(), 42);
    }
}
