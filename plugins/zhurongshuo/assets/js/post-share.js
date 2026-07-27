(function($) {
    'use strict';

    const PostShare = {
        config: {
            cardWidth: 500,
            excerptLength: 300,
            qrCodeSize: 100
        },

        init: function() {
            if ($('.post_detail_title').length === 0) return;
            this.bindEvents();
        },

        bindEvents: function() {
            const self = this;
            $(document).on('click', '.share_post_btn', function(e) {
                e.preventDefault();
                self.generateShareCard();
            });
            $(document).on('click', '.share-modal-close, .share-modal', function(e) {
                if (e.target === this) self.closeModal();
            });
            $(document).on('click', '.share-download-btn', function(e) {
                e.preventDefault();
                self.downloadImage();
            });
        },

        generateShareCard: function() {
            const self = this;
            self.showLoading();

            const data = self.getPostData();

            // 追踪分享卡片生成
            if (window.ZrsAnalytics) {
                ZrsAnalytics.track('zrs_share_generate', {
                    share_title: data.title.substring(0, 100)
                });
            }

            const cardHTML = self.buildCardHTML(data);

            const $tempContainer = $('<div>').css({
                position: 'absolute',
                left: '-9999px',
                width: self.config.cardWidth + 'px',
                background: '#ffffff',
                fontFamily: '"Helvetica Neue", Arial, sans-serif'
            });

            $tempContainer.html(cardHTML);
            $('body').append($tempContainer);

            // Generate QR code and capture
            setTimeout(function() {
                const qrContainer = document.getElementById('share-qrcode-temp');
                new QRCode(qrContainer, {
                    text: data.url,
                    width: self.config.qrCodeSize,
                    height: self.config.qrCodeSize,
                    colorDark: '#1f1f1f',
                    colorLight: '#ffffff'
                });

                setTimeout(function() {
                    self.captureCard($tempContainer, data.title);
                }, 300);
            }, 100);
        },

        getPostData: function() {
            const self = this;
            let excerpt = '';

            const metaDesc = $('meta[name="description"]').attr('content');
            if (metaDesc) {
                excerpt = metaDesc;
            } else {
                const $firstP = $('.post_content.markdown p, .post_content p').first();
                if ($firstP.length) excerpt = $firstP.text().trim();
            }

            if (excerpt.length > this.config.excerptLength) {
                excerpt = excerpt.substring(0, this.config.excerptLength) + '...';
            }

            return {
                title: $('.post_detail_title h2 a').text().trim(),
                date: $('.post_detail_title .date').text().trim(),
                url: window.location.href,
                siteTitle: '祝融说。',
                excerpt: excerpt
            };
        },

        buildCardHTML: function(data) {
            return `
                <div class="share-card" style="padding:30px;background:#fff;">
                    <div class="share-card-header" style="display:flex;align-items:center;justify-content:center;margin-bottom:25px;padding-bottom:15px;border-bottom:2px solid #94352d;">
                        <img src="/images/logo.png" style="width:32px;height:32px;margin-right:8px;" />
                        <span class="share-card-logo" style="font-size:28px;font-weight:bold;color:#1f1f1f;">${data.siteTitle}</span>
                    </div>
                    <div style="text-align:center;margin-bottom:20px;">
                        <span class="share-card-url" style="font-size:12px;color:#bbbbbb;">zhurongshuo.com</span>
                    </div>
                    <div class="share-card-content">
                        <h1 style="font-size:28px;color:#1f1f1f;line-height:1.4;margin:0 0 10px;">${this.escapeHtml(data.title)}</h1>
                        <div style="font-size:14px;color:rgba(0,0,0,0.44);margin-bottom:20px;">${data.date}</div>
                        <div style="font-size:16px;line-height:1.8;color:#333;">${this.escapeHtml(data.excerpt)}</div>
                    </div>
                    <div class="share-card-footer" style="margin-top:30px;text-align:center;padding-top:20px;border-top:1px solid #f3f3f3;">
                        <div id="share-qrcode-temp" style="display:flex;justify-content:center;margin-bottom:10px;"></div>
                        <p style="font-size:14px;color:#888;margin:5px 0;">扫码阅读全文</p>
                    </div>
                </div>
            `;
        },

        captureCard: function($container, title) {
            const self = this;
            html2canvas($container.find('.share-card')[0], {
                scale: 2,
                useCORS: true,
                backgroundColor: '#ffffff',
                logging: false
            }).then(function(canvas) {
                $container.remove();
                self.hideLoading();
                self.showPreview(canvas, title);
            }).catch(function(error) {
                console.error('Generation failed:', error);
                $container.remove();
                self.hideLoading();
                alert('生成分享卡片失败，请重试');
            });
        },

        showPreview: function(canvas, title) {
            const dataURL = canvas.toDataURL('image/png');
            $('#share-preview-img').attr('src', dataURL);
            this.currentImageURL = dataURL;
            this.currentTitle = title;
            $('.share-modal').fadeIn(200);
        },

        downloadImage: function() {
            if (!this.currentImageURL) return;

            // 追踪分享图片下载
            if (window.ZrsAnalytics) {
                ZrsAnalytics.track('zrs_share_download', {
                    share_title: (this.currentTitle || '').substring(0, 100)
                });
            }

            const link = document.createElement('a');
            link.download = `祝融说-${this.currentTitle}.png`;
            link.href = this.currentImageURL;
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
        },

        closeModal: function() {
            $('.share-modal').fadeOut(200);
        },

        showLoading: function() {
            $('.share-loading').fadeIn(200);
        },

        hideLoading: function() {
            $('.share-loading').fadeOut(200);
        },

        escapeHtml: function(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
    };

    $(document).ready(function() {
        PostShare.init();
    });
})(jQuery);
