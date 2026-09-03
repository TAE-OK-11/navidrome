use std::time::{Duration, Instant};

const FAIL_THRESHOLD: u32 = 5;
const OPEN_FOR: Duration = Duration::from_secs(30);

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum State {
    Closed,
    Open,
    HalfOpen,
}

#[derive(Debug)]
pub struct CircuitBreaker {
    state: State,
    consecutive_fails: u32,
    opened_at: Option<Instant>,
}

impl Default for CircuitBreaker {
    fn default() -> Self {
        Self {
            state: State::Closed,
            consecutive_fails: 0,
            opened_at: None,
        }
    }
}

impl CircuitBreaker {
    pub fn allow(&mut self) -> bool {
        match self.state {
            State::Open => {
                if self
                    .opened_at
                    .is_some_and(|opened| opened.elapsed() >= OPEN_FOR)
                {
                    self.state = State::HalfOpen;
                    true
                } else {
                    false
                }
            }
            State::HalfOpen | State::Closed => true,
        }
    }

    pub fn success(&mut self) {
        self.consecutive_fails = 0;
        self.state = State::Closed;
        self.opened_at = None;
    }

    pub fn failure(&mut self) {
        self.consecutive_fails += 1;
        if self.state == State::HalfOpen || self.consecutive_fails >= FAIL_THRESHOLD {
            self.state = State::Open;
            self.opened_at = Some(Instant::now());
        }
    }

    pub fn is_open(&self) -> bool {
        self.state == State::Open
            && self
                .opened_at
                .is_some_and(|opened| opened.elapsed() < OPEN_FOR)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn opens_after_threshold() {
        let mut breaker = CircuitBreaker::default();
        for _ in 0..FAIL_THRESHOLD {
            assert!(breaker.allow());
            breaker.failure();
        }
        assert!(breaker.is_open());
        assert!(!breaker.allow());
    }

    #[test]
    fn success_closes() {
        let mut breaker = CircuitBreaker::default();
        for _ in 0..FAIL_THRESHOLD {
            breaker.failure();
        }
        breaker.success();
        assert!(!breaker.is_open());
        assert!(breaker.allow());
    }
}
