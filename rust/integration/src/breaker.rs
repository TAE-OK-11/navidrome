use std::time::{Duration, Instant};

const FAIL_THRESHOLD: u32 = 5;
const OPEN_FOR: Duration = Duration::from_secs(30);
const HALF_OPEN_MAX: u32 = 1;

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
    half_open_in_flight: u32,
}

impl Default for CircuitBreaker {
    fn default() -> Self {
        Self {
            state: State::Closed,
            consecutive_fails: 0,
            opened_at: None,
            half_open_in_flight: 0,
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
                    self.half_open_in_flight = 1;
                    true
                } else {
                    false
                }
            }
            State::HalfOpen => {
                if self.half_open_in_flight >= HALF_OPEN_MAX {
                    return false;
                }
                self.half_open_in_flight += 1;
                true
            }
            State::Closed => true,
        }
    }

    pub fn success(&mut self) {
        self.consecutive_fails = 0;
        self.half_open_in_flight = 0;
        self.state = State::Closed;
        self.opened_at = None;
    }

    pub fn failure(&mut self) {
        self.consecutive_fails += 1;
        if self.state == State::HalfOpen || self.consecutive_fails >= FAIL_THRESHOLD {
            self.state = State::Open;
            self.opened_at = Some(Instant::now());
            self.half_open_in_flight = 0;
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

    #[test]
    fn half_open_allows_single_probe() {
        let mut breaker = CircuitBreaker::default();
        for _ in 0..FAIL_THRESHOLD {
            breaker.failure();
        }
        breaker.opened_at = Some(Instant::now() - OPEN_FOR - Duration::from_secs(1));
        assert!(breaker.allow());
        assert!(!breaker.allow());
    }
}
