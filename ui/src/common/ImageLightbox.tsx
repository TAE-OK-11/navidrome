import Lightbox from 'yet-another-react-lightbox'
import 'yet-another-react-lightbox/styles.css'

type ImageLightboxProps = {
  open: boolean
  imageUrl: string
  title?: string
  onClose: () => void
}

const ImageLightbox = ({ open, imageUrl, title, onClose }: ImageLightboxProps) => (
  <Lightbox
    open={open}
    close={onClose}
    slides={[{ src: imageUrl, alt: title }]}
    animation={{ fade: 200 }}
    controller={{ closeOnBackdropClick: true }}
    styles={{ container: { padding: '50px' } }}
  />
)

export default ImageLightbox
