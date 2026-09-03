pub mod proto {
    tonic::include_proto!("navidrome.integration.v1");
}

pub mod breaker;
pub mod outbound;
pub mod sign;

pub use breaker::CircuitBreaker;
pub use outbound::OutboundService;
pub use sign::sign_audioscrobbler;
